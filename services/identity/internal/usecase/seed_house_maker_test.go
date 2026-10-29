package usecase_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/usecase"
)

func Test_SeedHouseMaker_Upserts(t *testing.T) {
	d := newDeps(t)

	d.ledger.EXPECT().ResolveParty(Any, "otc-desk-maker::1220ab").Return("otc-desk-maker::1220ab", nil)

	var upserted domain.User

	d.postgres.EXPECT().UpsertHouseUser(Any, Any).
		Run(func(_ context.Context, u domain.User) { upserted = u }).
		Return(nil)

	require.NoError(t, d.useCase().SeedHouseMaker(t.Context()))

	require.Equal(t, domain.KindHouse, upserted.Kind)
	require.Equal(t, "otc-desk-maker", upserted.Hint)
	require.Equal(t, "otc-desk-maker", upserted.DisplayName)
	require.Equal(t, "otc-desk-maker::1220ab", upserted.PartyID)
	require.True(t, upserted.MakerMode)
	require.Equal(t, domain.HashAPIKey("maker-key"), upserted.APIKeyHash)
	require.Empty(t, upserted.CredentialID)
}

func Test_SeedHouseMaker_WaitsForTheParty(t *testing.T) {
	d := newDeps(t)

	d.ledger.EXPECT().ResolveParty(Any, "otc-desk-maker::1220ab").Return("", domain.ErrNotFound)

	err := d.useCase().SeedHouseMaker(t.Context())
	require.ErrorIs(t, err, domain.ErrNotFound, "the caller retries until the maker is in")
}

func Test_SeedHouseMaker_NoMakerConfigured(t *testing.T) {
	d := newDeps(t)

	u := usecase.New(d.postgres, d.redis, d.ledger, usecase.Config{})

	require.NoError(t, u.SeedHouseMaker(t.Context()))
}

func Test_SeedHouseMaker_AFailedUpsertIsNotASeed(t *testing.T) {
	d := newDeps(t)

	taken := errors.New("hint belongs to a wallet")

	d.ledger.EXPECT().ResolveParty(Any, "otc-desk-maker::1220ab").Return("otc-desk-maker::1220ab", nil)
	d.postgres.EXPECT().UpsertHouseUser(Any, Any).Return(taken)

	require.ErrorIs(t, d.useCase().SeedHouseMaker(t.Context()), taken)
}
