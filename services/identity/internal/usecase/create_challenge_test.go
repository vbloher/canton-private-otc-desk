package usecase_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/dto"
)

func Test_CreateChallenge_StoresAOneShotChallenge(t *testing.T) {
	d := newDeps(t)

	user, _ := walletUser(t)

	var (
		saved   string
		savedTo uuid.UUID
		ttl     time.Duration
	)

	d.postgres.EXPECT().GetUserByCredential(Any, "cred-1").Return(user, nil)
	d.redis.EXPECT().SaveChallenge(Any, Any, Any, Any).
		Run(func(_ context.Context, challenge string, userID uuid.UUID, t time.Duration) {
			saved, savedTo, ttl = challenge, userID, t
		}).
		Return(nil)

	output, err := d.useCase().CreateChallenge(t.Context(), dto.CreateChallengeInput{CredentialID: "cred-1"})
	require.NoError(t, err)

	require.Regexp(t, `^otc-desk:`, output.Challenge)
	require.Equal(t, saved, output.Challenge)
	require.Equal(t, user.ID, savedTo, "the challenge belongs to the wallet that asked for it")
	require.Equal(t, 5*time.Minute, ttl)
	require.Equal(t, user.EncryptedBlob, output.EncryptedBlob)
	require.Equal(t, user.PublicKey, output.PublicKey)
}

func Test_CreateChallenge_UnknownCredential(t *testing.T) {
	d := newDeps(t)

	d.postgres.EXPECT().GetUserByCredential(Any, "nope").Return(domain.User{}, domain.ErrNotFound)

	_, err := d.useCase().CreateChallenge(t.Context(), dto.CreateChallengeInput{CredentialID: "nope"})
	require.ErrorIs(t, err, domain.ErrNotFound)
}

func Test_CreateChallenge_ByParty(t *testing.T) {
	d := newDeps(t)

	user, _ := keyAccount(t)

	d.postgres.EXPECT().GetUserByParty(Any, user.PartyID).Return(user, nil)
	d.redis.EXPECT().SaveChallenge(Any, Any, user.ID, 5*time.Minute).Return(nil)

	output, err := d.useCase().CreateChallenge(t.Context(), dto.CreateChallengeInput{PartyID: user.PartyID})
	require.NoError(t, err)

	require.Regexp(t, `^otc-desk:`, output.Challenge)
	require.Empty(t, output.EncryptedBlob)
	require.Equal(t, user.PublicKey, output.PublicKey)
}

func Test_CreateChallenge_ByPartyOfAPasskeyWallet(t *testing.T) {
	d := newDeps(t)

	user, _ := walletUser(t)

	d.postgres.EXPECT().GetUserByParty(Any, user.PartyID).Return(user, nil)

	_, err := d.useCase().CreateChallenge(t.Context(), dto.CreateChallengeInput{PartyID: user.PartyID})
	require.ErrorIs(t, err, domain.ErrNotFound, "a wrapped key never leaves by party id")
}

func Test_CreateChallenge_NeedsExactlyOneIdentifier(t *testing.T) {
	d := newDeps(t)

	for name, input := range map[string]dto.CreateChallengeInput{
		"neither": {},
		"both":    {CredentialID: "cred-1", PartyID: "demo-maker-1a2b3c::1220ab"},
	} {
		_, err := d.useCase().CreateChallenge(t.Context(), input)
		require.True(t, domain.IsInvalidInput(err), "%s: %v", name, err)
	}
}
