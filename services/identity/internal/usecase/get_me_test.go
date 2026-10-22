package usecase_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/dto"
)

func Test_GetMe_Success(t *testing.T) {
	d := newDeps(t)

	user, _ := walletUser(t)

	d.postgres.EXPECT().GetUser(Any, user.ID).Return(user, nil)

	output, err := d.useCase().GetMe(t.Context(), dto.GetMeInput{UserID: user.ID})
	require.NoError(t, err)
	require.Equal(t, dto.GetMeOutput{User: user}, output)
}

func Test_GetMe_NotFound(t *testing.T) {
	d := newDeps(t)

	id := uuid.New()

	d.postgres.EXPECT().GetUser(Any, id).Return(domain.User{}, domain.ErrNotFound)

	_, err := d.useCase().GetMe(t.Context(), dto.GetMeInput{UserID: id})
	require.ErrorIs(t, err, domain.ErrNotFound)
}
