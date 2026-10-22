package usecase_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/dto"
)

func Test_SetMakerMode_Success(t *testing.T) {
	d := newDeps(t)

	user, _ := walletUser(t)
	user.MakerMode = true

	d.postgres.EXPECT().SetMakerMode(Any, user.ID, true).Return(user, nil)

	output, err := d.useCase().SetMakerMode(t.Context(), dto.SetMakerModeInput{UserID: user.ID, Enabled: true})
	require.NoError(t, err)
	require.Equal(t, dto.SetMakerModeOutput{User: user}, output)
}

func Test_SetMakerMode_NotFound(t *testing.T) {
	d := newDeps(t)

	id := uuid.New()

	d.postgres.EXPECT().SetMakerMode(Any, id, false).Return(domain.User{}, domain.ErrNotFound)

	_, err := d.useCase().SetMakerMode(t.Context(), dto.SetMakerModeInput{UserID: id, Enabled: false})
	require.ErrorIs(t, err, domain.ErrNotFound)
}
