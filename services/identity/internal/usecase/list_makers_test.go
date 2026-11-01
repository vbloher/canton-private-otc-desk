package usecase_test

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/dto"
)

func maker(hint string) domain.User {
	return domain.User{ID: uuid.New(), PartyID: hint + "::1220ab", Hint: hint, DisplayName: hint, MakerMode: true}
}

func Test_ListMakers_ExcludesTheCaller(t *testing.T) {
	d := newDeps(t)

	makerA, makerB, alice := maker("makerA"), maker("makerB"), maker("alice-3f9a1c")

	d.postgres.EXPECT().ListMakers(Any).Return([]domain.User{makerA, alice, makerB}, nil)
	d.redis.EXPECT().Online(Any, []string{makerA.PartyID, alice.PartyID, makerB.PartyID}).
		Return(map[string]bool{makerA.PartyID: true, alice.PartyID: true, makerB.PartyID: true}, nil)

	output, err := d.useCase().ListMakers(t.Context(), dto.ListMakersInput{CallerID: alice.ID})
	require.NoError(t, err)
	require.Equal(t, []domain.User{makerA, makerB}, output.Makers)
}

func Test_ListMakers_OnlyOnline(t *testing.T) {
	d := newDeps(t)

	makerA, makerB := maker("makerA"), maker("makerB")

	d.postgres.EXPECT().ListMakers(Any).Return([]domain.User{makerA, makerB}, nil)
	d.redis.EXPECT().Online(Any, []string{makerA.PartyID, makerB.PartyID}).
		Return(map[string]bool{makerA.PartyID: true}, nil)

	output, err := d.useCase().ListMakers(t.Context(), dto.ListMakersInput{CallerID: uuid.New()})
	require.NoError(t, err)
	require.Equal(t, []domain.User{makerA}, output.Makers, "a maker without a live connection is not offered")
}

func Test_ListMakers_RedisFails(t *testing.T) {
	d := newDeps(t)

	makerA := maker("makerA")

	d.postgres.EXPECT().ListMakers(Any).Return([]domain.User{makerA}, nil)
	d.redis.EXPECT().Online(Any, []string{makerA.PartyID}).Return(nil, errors.New("redis is down"))

	_, err := d.useCase().ListMakers(t.Context(), dto.ListMakersInput{CallerID: uuid.New()})
	require.Error(t, err)
}

func Test_ListMakers_NoMakers(t *testing.T) {
	d := newDeps(t)

	d.postgres.EXPECT().ListMakers(Any).Return(nil, nil)

	output, err := d.useCase().ListMakers(t.Context(), dto.ListMakersInput{CallerID: uuid.New()})
	require.NoError(t, err)
	require.Empty(t, output.Makers)
}
