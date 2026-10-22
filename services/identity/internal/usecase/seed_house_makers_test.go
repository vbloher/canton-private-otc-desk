package usecase_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/usecase"
)

func Test_SeedHouseMakers_Upserts(t *testing.T) {
	d := newDeps(t)

	d.ledger.EXPECT().ResolveParty(Any, "makerA").Return("makerA::1220ab", nil)
	d.ledger.EXPECT().ResolveParty(Any, "makerB").Return("makerB::1220ab", nil)

	var upserted []domain.User

	d.postgres.EXPECT().UpsertHouseUser(Any, Any).
		Run(func(_ context.Context, u domain.User) { upserted = append(upserted, u) }).
		Return(nil)

	require.NoError(t, d.useCase().SeedHouseMakers(t.Context()))
	require.Len(t, upserted, 2)

	for i, want := range []struct{ hint, partyID, apiKey string }{
		{"makerA", "makerA::1220ab", "demo-key-a"},
		{"makerB", "makerB::1220ab", "demo-key-b"},
	} {
		require.Equal(t, domain.KindHouse, upserted[i].Kind)
		require.Equal(t, want.hint, upserted[i].Hint)
		require.Equal(t, want.hint, upserted[i].DisplayName)
		require.Equal(t, want.partyID, upserted[i].PartyID)
		require.True(t, upserted[i].MakerMode)
		require.Equal(t, domain.HashAPIKey(want.apiKey), upserted[i].APIKeyHash)
		require.Empty(t, upserted[i].CredentialID)
	}
}

func Test_SeedHouseMakers_KeepsGoingAfterAFailure(t *testing.T) {
	d := newDeps(t)

	d.ledger.EXPECT().ResolveParty(Any, "makerA").Return("", domain.ErrNotFound)
	d.ledger.EXPECT().ResolveParty(Any, "makerB").Return("makerB::1220ab", nil)

	var upserted []domain.User

	d.postgres.EXPECT().UpsertHouseUser(Any, Any).
		Run(func(_ context.Context, u domain.User) { upserted = append(upserted, u) }).
		Return(nil)

	err := d.useCase().SeedHouseMakers(t.Context())
	require.ErrorIs(t, err, domain.ErrNotFound, "the caller retries until every maker is in")
	require.Len(t, upserted, 1)
	require.Equal(t, "makerB", upserted[0].Hint, "one missing party does not hold back the others")
}

func Test_SeedHouseMakers_NoMakersConfigured(t *testing.T) {
	d := newDeps(t) // no expectations

	u := usecase.New(d.postgres, d.redis, d.ledger, usecase.Config{})

	require.NoError(t, u.SeedHouseMakers(t.Context()))
}

// An upsert the database refused (a hint that belongs to a wallet, say) is not a seed: the error
// reaches the caller, which logs it and tries again instead of reporting the house makers as seeded.
func Test_SeedHouseMakers_AFailedUpsertIsNotASeed(t *testing.T) {
	d := newDeps(t)

	taken := errors.New("hint belongs to a wallet")

	d.ledger.EXPECT().ResolveParty(Any, "makerA").Return("makerA::1220ab", nil)
	d.ledger.EXPECT().ResolveParty(Any, "makerB").Return("makerB::1220ab", nil)
	d.postgres.EXPECT().UpsertHouseUser(Any, mock.MatchedBy(func(u domain.User) bool { return u.Hint == "makerA" })).
		Return(taken)
	d.postgres.EXPECT().UpsertHouseUser(Any, mock.MatchedBy(func(u domain.User) bool { return u.Hint == "makerB" })).
		Return(nil)

	err := d.useCase().SeedHouseMakers(t.Context())
	require.ErrorIs(t, err, taken)
	require.ErrorContains(t, err, "makerA")
}
