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
