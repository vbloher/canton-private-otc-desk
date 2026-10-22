package domain_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/pkg/decimal"
	"github.com/vbloher/canton-private-otc-desk/services/maker-bot/internal/domain"
)

func usd() map[string]decimal.Decimal {
	return map[string]decimal.Decimal{
		"CBTC":  decimal.MustParse("100000"),
		"CETH":  decimal.MustParse("4000"),
		"USDCx": decimal.MustParse("1"),
	}
}

func Test_Price_TakerBuyAddsSpread(t *testing.T) {
	price, err := domain.Price("BUY", "CBTC", "USDCx", usd(), 15)
	require.NoError(t, err)
	require.Equal(t, "100150.0", price.String())
}

func Test_Price_TakerSellSubtractsSpread(t *testing.T) {
	price, err := domain.Price("SELL", "CBTC", "USDCx", usd(), 15)
	require.NoError(t, err)
	require.Equal(t, "99850.0", price.String())
}

func Test_Price_CrossAndInverse(t *testing.T) {
	price, err := domain.Price("BUY", "CBTC", "CETH", usd(), 0)
	require.NoError(t, err)
	require.Equal(t, "25.0", price.String())

	price, err = domain.Price("SELL", "USDCx", "CBTC", usd(), 40)
	require.NoError(t, err)
	require.Equal(t, "0.00000996", price.String())
}

func Test_Price_UnknownInstrument(t *testing.T) {
	_, err := domain.Price("BUY", "DOGE", "USDCx", usd(), 15)
	require.ErrorIs(t, err, domain.ErrUnknownInstrument)

	_, err = domain.Price("BUY", "CBTC", "DOGE", usd(), 15)
	require.ErrorIs(t, err, domain.ErrUnknownInstrument)
}

func Test_Price_Rejects(t *testing.T) {
	_, err := domain.Price("HOLD", "CBTC", "USDCx", usd(), 15)
	require.ErrorIs(t, err, domain.ErrInvalidPrice)

	_, err = domain.Price("SELL", "CBTC", "USDCx", usd(), 10_000)
	require.ErrorIs(t, err, domain.ErrInvalidPrice, "a spread of 100% leaves nothing to quote")
}

func Test_Prices_UnmarshalText(t *testing.T) {
	var prices domain.Prices

	require.NoError(t, prices.UnmarshalText([]byte("CBTC:100000,CETH:4000,USDCx:1")))
	require.Len(t, prices, 3)
	require.Equal(t, "4000.0", prices["CETH"].String())

	for _, bad := range []string{"CBTC", "CBTC:abc", "CBTC:0", ":1", "CBTC:1,CBTC:2"} {
		require.Error(t, new(domain.Prices).UnmarshalText([]byte(bad)), bad)
	}
}

func Test_Makers_UnmarshalText(t *testing.T) {
	var makers domain.Makers

	require.NoError(t, makers.UnmarshalText([]byte("demo-key-a:15,demo-key-b:40")))
	require.Equal(t, domain.Makers{{APIKey: "demo-key-a", SpreadBps: 15}, {APIKey: "demo-key-b", SpreadBps: 40}}, makers)

	for _, bad := range []string{"demo-key-a", "demo-key-a:x", ":15", "demo-key-a:-1", "demo-key-a:10000"} {
		require.Error(t, new(domain.Makers).UnmarshalText([]byte(bad)), bad)
	}
}
