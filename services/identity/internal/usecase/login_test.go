package usecase_test

import (
	"context"
	"crypto/ed25519"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/dto"
)

const challenge = "otc-desk:4kJ8m2Pq"

func Test_Login_BadSignature(t *testing.T) {
	d := newDeps(t)

	user, _ := walletUser(t)

	// Well-formed, but signed by a key that is not the wallet's.
	_, otherKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	d.redis.EXPECT().TakeChallenge(Any, challenge).Return(user.ID, nil)
	d.postgres.EXPECT().GetUser(Any, user.ID).Return(user, nil)
	// No CreateSession expectation: a bad signature must not leave a session behind.

	output, err := d.useCase().Login(t.Context(), dto.LoginInput{
		Challenge:    challenge,
		Signature:    ed25519.Sign(otherKey, []byte(challenge)),
		CredentialID: "cred-1",
	})
	require.ErrorIs(t, err, domain.ErrUnauthorized)
	require.Equal(t, dto.LoginOutput{}, output)
}

func Test_Login_Success(t *testing.T) {
	d := newDeps(t)

	user, privateKey := walletUser(t)

	var (
		sessionHash   string
		sessionUserID uuid.UUID
		sessionExpiry time.Time
	)

	d.redis.EXPECT().TakeChallenge(Any, challenge).Return(user.ID, nil)
	d.postgres.EXPECT().GetUser(Any, user.ID).Return(user, nil)
	d.postgres.EXPECT().CreateSession(Any, Any, Any, Any).
		Run(func(_ context.Context, hash string, userID uuid.UUID, expiresAt time.Time) {
			sessionHash, sessionUserID, sessionExpiry = hash, userID, expiresAt
		}).
		Return(nil)

	before := time.Now()

	output, err := d.useCase().Login(t.Context(), dto.LoginInput{
		Challenge:    challenge,
		Signature:    ed25519.Sign(privateKey, []byte(challenge)),
		CredentialID: "cred-1",
	})
	require.NoError(t, err)

	after := time.Now()

	require.Equal(t, user, output.User)
	require.NotEmpty(t, output.Token)
	require.Equal(t, testConfig.SessionTTL, output.TTL)
	require.Equal(t, domain.HashSessionToken(output.Token), sessionHash, "only the hash of the token is stored")
	require.Equal(t, user.ID, sessionUserID)
	require.WithinRange(t, sessionExpiry, before.Add(testConfig.SessionTTL), after.Add(testConfig.SessionTTL))
}

func Test_Login_UnknownOrUsedChallenge(t *testing.T) {
	d := newDeps(t)

	_, privateKey := walletUser(t)

	d.redis.EXPECT().TakeChallenge(Any, challenge).Return(uuid.Nil, domain.ErrUnauthorized)

	_, err := d.useCase().Login(t.Context(), dto.LoginInput{
		Challenge:    challenge,
		Signature:    ed25519.Sign(privateKey, []byte(challenge)),
		CredentialID: "cred-1",
	})
	require.ErrorIs(t, err, domain.ErrUnauthorized)
}

func Test_Login_ChallengeOfAnotherCredential(t *testing.T) {
	d := newDeps(t)

	user, privateKey := walletUser(t)

	d.redis.EXPECT().TakeChallenge(Any, challenge).Return(user.ID, nil)
	d.postgres.EXPECT().GetUser(Any, user.ID).Return(user, nil)

	_, err := d.useCase().Login(t.Context(), dto.LoginInput{
		Challenge:    challenge,
		Signature:    ed25519.Sign(privateKey, []byte(challenge)),
		CredentialID: "cred-of-somebody-else",
	})
	require.ErrorIs(t, err, domain.ErrUnauthorized)
}

func Test_Login_UserGone(t *testing.T) {
	d := newDeps(t)

	id := uuid.New()

	d.redis.EXPECT().TakeChallenge(Any, challenge).Return(id, nil)
	d.postgres.EXPECT().GetUser(Any, id).Return(domain.User{}, domain.ErrNotFound)

	_, err := d.useCase().Login(t.Context(), dto.LoginInput{
		Challenge:    challenge,
		Signature:    make([]byte, 64),
		CredentialID: "cred-1",
	})
	require.ErrorIs(t, err, domain.ErrUnauthorized)
}

func Test_Login_MalformedSignatureKeepsTheChallenge(t *testing.T) {
	d := newDeps(t) // no expectations: the challenge must not be consumed

	_, err := d.useCase().Login(t.Context(), dto.LoginInput{
		Challenge:    challenge,
		Signature:    make([]byte, 10),
		CredentialID: "cred-1",
	})
	require.ErrorIs(t, err, domain.ErrValidation)
}
