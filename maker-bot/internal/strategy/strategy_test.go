package strategy_test

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/maker-bot/internal/strategy"
)

func rat(t *testing.T, s string) *big.Rat {
	t.Helper()

	r, ok := new(big.Rat).SetString(s)
	require.True(t, ok, s)

	return r
}

func prices(t *testing.T) map[string]*big.Rat {
	t.Helper()

	return map[string]*big.Rat{"CBTC": rat(t, "100000"), "USDCx": rat(t, "1"), "CC": rat(t, "0.15")}
}

func TestPrice(t *testing.T) {
	for _, c := range []struct {
		base, quote string
		takerBuys   bool
		want        string
	}{
		{"CBTC", "USDCx", true, "100150"},
		{"CBTC", "USDCx", false, "99850"},
		{"CC", "USDCx", true, "0.150225"},
		{"CC", "USDCx", false, "0.149775"},
		{"CBTC", "CC", true, "667666.6666666667"},
	} {
		mid, err := strategy.Mid(prices(t), c.base, c.quote)
		require.NoError(t, err)

		price, err := strategy.Price(mid, c.takerBuys, 15)
		require.NoError(t, err)
		require.Equal(t, c.want, strategy.Format(price), "%s/%s, taker buys: %v", c.base, c.quote, c.takerBuys)
	}
}

func TestMid_UnknownSymbol(t *testing.T) {
	_, err := strategy.Mid(prices(t), "ETHx", "USDCx")
	require.ErrorIs(t, err, strategy.ErrNoPrice)

	_, err = strategy.Mid(prices(t), "CBTC", "ETHx")
	require.ErrorIs(t, err, strategy.ErrNoPrice)
}

func TestPrice_RoundsToZero(t *testing.T) {
	_, err := strategy.Price(rat(t, "0.00000000001"), false, 15)
	require.ErrorIs(t, err, strategy.ErrNotPositive)
}

func TestSpend(t *testing.T) {
	base, quote := strategy.Spend(true, rat(t, "0.01"), rat(t, "100150"), 10)
	require.Equal(t, "0.01", strategy.Format(base), "a buying taker gets the base from the bot")
	require.Equal(t, "1.0015", strategy.Format(quote), "and the bot pays the fee in the quote asset")

	base, quote = strategy.Spend(false, rat(t, "0.01"), rat(t, "99850"), 10)
	require.Equal(t, "0", strategy.Format(base))
	require.Equal(t, "999.4985", strategy.Format(quote), "a selling taker gets the notional plus nothing, the bot adds the fee")
}

func TestFormat(t *testing.T) {
	for in, want := range map[string]string{
		"100150": "100150",
		"1.50":   "1.5",
		"1/3":    "0.3333333333",
		"2/3":    "0.6666666667",
		"0":      "0",
	} {
		require.Equal(t, want, strategy.Format(rat(t, in)), in)
	}
}
