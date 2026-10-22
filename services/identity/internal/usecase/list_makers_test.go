package usecase_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/dto"
)

func Test_ListMakers_ExcludesTheCaller(t *testing.T) {
	d := newDeps(t)

	makerA := domain.User{ID: uuid.New(), Hint: "makerA", DisplayName: "makerA", MakerMode: true}
	makerB := domain.User{ID: uuid.New(), Hint: "makerB", DisplayName: "makerB", MakerMode: true}
	alice := domain.User{ID: uuid.New(), Hint: "alice-3f9a1c", DisplayName: "Alice", MakerMode: true}

	d.postgres.EXPECT().ListMakers(Any).Return([]domain.User{makerA, alice, makerB}, nil)

	output, err := d.useCase().ListMakers(t.Context(), dto.ListMakersInput{CallerID: alice.ID})
	require.NoError(t, err)
	require.Equal(t, []domain.User{makerA, makerB}, output.Makers)
}

func Test_ListMakers_NoMakers(t *testing.T) {
	d := newDeps(t)

	d.postgres.EXPECT().ListMakers(Any).Return(nil, nil)

	output, err := d.useCase().ListMakers(t.Context(), dto.ListMakersInput{CallerID: uuid.New()})
	require.NoError(t, err)
	require.Empty(t, output.Makers)
}
