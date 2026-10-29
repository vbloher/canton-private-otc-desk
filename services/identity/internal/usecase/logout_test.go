package usecase_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/dto"
)

func Test_Logout_DeletesTheSessionAndItsCacheEntry(t *testing.T) {
	d := newDeps(t)

	hash := domain.HashSessionToken("token")

	d.postgres.EXPECT().DeleteSession(Any, hash).Return(nil)
	d.redis.EXPECT().DropCachedSession(Any, hash).Return()

	require.NoError(t, d.useCase().Logout(t.Context(), dto.LogoutInput{Token: "token"}))
}

func Test_Logout_WithoutACookieDoesNothing(t *testing.T) {
	d := newDeps(t)

	require.NoError(t, d.useCase().Logout(t.Context(), dto.LogoutInput{Token: ""}))
}

func Test_Logout_KeepsTheCacheWhenTheDeleteFails(t *testing.T) {
	d := newDeps(t)

	boom := errors.New("connection reset")

	d.postgres.EXPECT().DeleteSession(Any, domain.HashSessionToken("token")).Return(boom)

	err := d.useCase().Logout(t.Context(), dto.LogoutInput{Token: "token"})
	require.ErrorIs(t, err, boom)
}
