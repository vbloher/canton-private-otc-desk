package domain_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/pkg/decimal"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/domain"
)

func holding(cid, owner, instrument, amount string, locked bool) domain.Holding {
	return domain.Holding{
		ContractID:   cid,
		Owner:        owner,
		Admin:        "operator",
		InstrumentID: instrument,
		Amount:       decimal.MustParse(amount),
		Locked:       locked,
	}
}

func Test_SelectHoldings_LargestFirst(t *testing.T) {
	hs := []domain.Holding{
		holding("small", "alice", "CBTC", "1.0", false),
		holding("large", "alice", "CBTC", "5.0", false),
		holding("medium", "alice", "CBTC", "3.0", false),
	}

	cids, err := domain.SelectHoldings(hs, "alice", "operator", "CBTC", decimal.MustParse("6.0"), time.Now())
	require.NoError(t, err)
	require.Equal(t, []string{"large", "medium"}, cids)

	cids, err = domain.SelectHoldings(hs, "alice", "operator", "CBTC", decimal.MustParse("5.0"), time.Now())
	require.NoError(t, err)
	require.Equal(t, []string{"large"}, cids)
}

func Test_SelectHoldings_SkipsLockedAndForeign(t *testing.T) {
	foreignAdmin := holding("other-admin", "alice", "CBTC", "10.0", false)
	foreignAdmin.Admin = "someone-else"

	hs := []domain.Holding{
		holding("locked", "alice", "CBTC", "10.0", true),
		holding("not-mine", "bob", "CBTC", "10.0", false),
		holding("other-instrument", "alice", "CETH", "10.0", false),
		foreignAdmin,
		holding("mine", "alice", "CBTC", "2.0", false),
	}

	cids, err := domain.SelectHoldings(hs, "alice", "operator", "CBTC", decimal.MustParse("2.0"), time.Now())
	require.NoError(t, err)
	require.Equal(t, []string{"mine"}, cids)
}

func Test_SelectHoldings_Insufficient(t *testing.T) {
	hs := []domain.Holding{
		holding("a", "alice", "CBTC", "1.0", false),
		holding("b", "alice", "CBTC", "2.0", false),
		holding("locked", "alice", "CBTC", "9.0", true),
	}

	cids, err := domain.SelectHoldings(hs, "alice", "operator", "CBTC", decimal.MustParse("5.0"), time.Now())
	require.ErrorIs(t, err, domain.ErrInsufficientFunds)
	require.EqualError(t, err, "insufficient funds: CBTC: need 5.0, held 3.0")
	require.Nil(t, cids)
}

func Test_Balances_ZeroFilled(t *testing.T) {
	instruments := []domain.Instrument{
		{Symbol: "CBTC", Name: "Bitcoin", Decimals: 8, ID: "CBTC", Admin: "operator"},
		{Symbol: "USDCx", Name: "USD Coin", Decimals: 2, ID: "USDCx", Admin: "operator"},
		{Symbol: "CC", Name: "Canton Coin", Decimals: 6, ID: "Amulet", Admin: "operator"},
	}
	foreignAdmin := holding("f", "alice", "CBTC", "50.0", false)
	foreignAdmin.Admin = "someone-else"

	hs := []domain.Holding{
		holding("a", "alice", "CBTC", "1.5", false),
		holding("b", "alice", "CBTC", "0.5", true),
		holding("c", "bob", "CBTC", "9.0", false),
		holding("d", "alice", "USDCx", "100.0", false),
		holding("e", "alice", "UNKNOWN", "7.0", false),
		foreignAdmin,
	}

	balances := domain.Balances(hs, "alice", instruments, time.Now())

	got := make([][3]string, 0, len(balances))
	for _, b := range balances {
		got = append(got, [3]string{b.Symbol, b.Available.String(), b.Locked.String()})
	}

	require.Equal(t, [][3]string{
		{"CBTC", "1.5", "0.5"},
		{"USDCx", "100.0", "0.0"},
		{"CC", "0.0", "0.0"},
	}, got)
}

func lapsedLocks(now time.Time) []domain.Holding {
	lapsed := holding("lapsed", "alice", "CBTC", "4.0", true)
	lapsed.LockedUntil = now.Add(-time.Minute)

	live := holding("live", "alice", "CBTC", "8.0", true)
	live.LockedUntil = now.Add(time.Minute)

	forever := holding("forever", "alice", "CBTC", "16.0", true)

	return []domain.Holding{lapsed, live, forever, holding("free", "alice", "CBTC", "1.0", false)}
}

func Test_SelectHoldings_LapsedLockIsSpendable(t *testing.T) {
	now := time.Now()

	cids, err := domain.SelectHoldings(lapsedLocks(now), "alice", "operator", "CBTC", decimal.MustParse("5.0"), now)
	require.NoError(t, err)
	require.Equal(t, []string{"lapsed", "free"}, cids)

	_, err = domain.SelectHoldings(lapsedLocks(now), "alice", "operator", "CBTC", decimal.MustParse("5.5"), now)
	require.EqualError(t, err, "insufficient funds: CBTC: need 5.5, held 5.0")

	hs := lapsedLocks(now)
	require.False(t, hs[0].LockedAt(hs[0].LockedUntil))
	require.True(t, hs[0].LockedAt(hs[0].LockedUntil.Add(-time.Microsecond)))
}

func Test_Balances_LapsedLockIsAvailable(t *testing.T) {
	now := time.Now()

	b := domain.Balances(lapsedLocks(now), "alice", []domain.Instrument{{Symbol: "CBTC", ID: "CBTC", Admin: "operator"}},
		now)[0]
	require.Equal(t, "5.0", b.Available.String())
	require.Equal(t, "24.0", b.Locked.String())
}
