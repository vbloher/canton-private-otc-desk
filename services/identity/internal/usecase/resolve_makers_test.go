package usecase_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/dto"
)

func Test_ResolveMakers_LeavesUnknownHintsOut(t *testing.T) {
	d := newDeps(t)

	makerA, makerB, bob := maker("makerA"), maker("makerB"), maker("bob-aaaaaa")

	d.postgres.EXPECT().ListMakers(Any).Return([]domain.User{makerA, makerB, bob}, nil)
	d.redis.EXPECT().Online(Any, []string{makerA.PartyID, makerB.PartyID, bob.PartyID}).
		Return(map[string]bool{makerA.PartyID: true, makerB.PartyID: true, bob.PartyID: true}, nil)

	output, err := d.useCase().ResolveMakers(t.Context(), dto.ResolveMakersInput{
		Hints: []string{"makerB", "ghost", "makerA", "makerB"},
	})
	require.NoError(t, err)
	require.Equal(t, []domain.User{makerB, makerA}, output.Makers, "in request order, each maker once")
}

func Test_ResolveMakers_LeavesOfflineMakersOut(t *testing.T) {
	d := newDeps(t)

	makerA, makerB := maker("makerA"), maker("makerB")

	d.postgres.EXPECT().ListMakers(Any).Return([]domain.User{makerA, makerB}, nil)
	d.redis.EXPECT().Online(Any, []string{makerA.PartyID, makerB.PartyID}).
		Return(map[string]bool{makerA.PartyID: true}, nil)

	output, err := d.useCase().ResolveMakers(t.Context(), dto.ResolveMakersInput{Hints: []string{"makerA", "makerB"}})
	require.NoError(t, err)
	require.Equal(t, []domain.User{makerA}, output.Makers)
}

func Test_ResolveMakers_NoHints(t *testing.T) {
	d := newDeps(t)

	output, err := d.useCase().ResolveMakers(t.Context(), dto.ResolveMakersInput{Hints: nil})
	require.NoError(t, err)
	require.Empty(t, output.Makers)
}

func Test_ResolveMakers_PostgresFails(t *testing.T) {
	d := newDeps(t)

	d.postgres.EXPECT().ListMakers(Any).Return(nil, domain.ErrNotFound)

	_, err := d.useCase().ResolveMakers(t.Context(), dto.ResolveMakersInput{Hints: []string{"makerA"}})
	require.Error(t, err)
}
