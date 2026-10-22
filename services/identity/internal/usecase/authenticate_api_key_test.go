package usecase_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/dto"
)

func Test_AuthenticateAPIKey_Success(t *testing.T) {
	d := newDeps(t)

	maker, err := domain.NewHouseUser("makerA::1220ab", "makerA", "demo-key-a")
	require.NoError(t, err)

	d.postgres.EXPECT().GetUserByAPIKeyHash(Any, domain.HashAPIKey("demo-key-a")).Return(maker, nil)

	output, err := d.useCase().AuthenticateAPIKey(t.Context(), dto.AuthenticateAPIKeyInput{APIKey: "demo-key-a"})
	require.NoError(t, err)
	require.Equal(t, dto.AuthenticateAPIKeyOutput{User: maker}, output)
}

func Test_AuthenticateAPIKey_Unknown(t *testing.T) {
	d := newDeps(t)

	d.postgres.EXPECT().GetUserByAPIKeyHash(Any, domain.HashAPIKey("guess")).Return(domain.User{}, domain.ErrNotFound)

	_, err := d.useCase().AuthenticateAPIKey(t.Context(), dto.AuthenticateAPIKeyInput{APIKey: "guess"})
	require.ErrorIs(t, err, domain.ErrUnauthorized)
}

func Test_AuthenticateAPIKey_Empty(t *testing.T) {
	d := newDeps(t) // no expectations

	_, err := d.useCase().AuthenticateAPIKey(t.Context(), dto.AuthenticateAPIKeyInput{APIKey: ""})
	require.ErrorIs(t, err, domain.ErrUnauthorized)
}
