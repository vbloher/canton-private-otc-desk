package usecase_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/dto"
)

func Test_ResolveMakers_LeavesUnknownHintsOut(t *testing.T) {
	d := newDeps(t)

	makerA := domain.User{ID: uuid.New(), Hint: "makerA", MakerMode: true}
	makerB := domain.User{ID: uuid.New(), Hint: "makerB", MakerMode: true}
	bob := domain.User{ID: uuid.New(), Hint: "bob-aaaaaa", MakerMode: true}

	d.postgres.EXPECT().ListMakers(Any).Return([]domain.User{makerA, makerB, bob}, nil)

	output, err := d.useCase().ResolveMakers(t.Context(), dto.ResolveMakersInput{
		Hints: []string{"makerB", "ghost", "makerA", "makerB"},
	})
	require.NoError(t, err)
	require.Equal(t, []domain.User{makerB, makerA}, output.Makers, "in request order, each maker once")
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
