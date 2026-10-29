package domain_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/domain"
)

const (
	cbtcEntry = "CBTC|Bitcoin|8|CBTC|cbtc-network::1220aa|" +
		"https://api.utilities.example/api/token-standard/v0/registrars/cbtc-network::1220aa/registry/"
	ccEntry = " CC | Canton Coin | 4 | Amulet | DSO::1220bb | https://scan.example/registry "
)

func Test_InstrumentList_Parse(t *testing.T) {
	var list domain.InstrumentList

	require.NoError(t, list.UnmarshalText([]byte(cbtcEntry+","+ccEntry)))
	require.Equal(t, domain.InstrumentList{
		{
			Symbol: "CBTC", Name: "Bitcoin", Decimals: 8, ID: "CBTC", Admin: "cbtc-network::1220aa",
			Registry: "https://api.utilities.example/api/token-standard/v0/registrars/cbtc-network::1220aa/registry",
		},
		{
			Symbol: "CC", Name: "Canton Coin", Decimals: 4, ID: "Amulet", Admin: "DSO::1220bb",
			Registry: "https://scan.example/registry",
		},
	}, list)

	cc, ok := list.Of("DSO::1220bb", "Amulet")
	require.True(t, ok)
	require.Equal(t, "CC", cc.Symbol)

	_, ok = list.Of("DSO::1220bb", "CBTC")
	require.False(t, ok)

	registry, ok := list.Registry("cbtc-network::1220aa")
	require.True(t, ok)
	require.Equal(t, list[0].Registry, registry)
}

func Test_InstrumentList_Invalid(t *testing.T) {
	for name, text := range map[string]string{
		"too few fields":    "CBTC|Bitcoin|8",
		"empty field":       "CBTC||8|CBTC|a::1|https://r",
		"bad decimals":      "CBTC|Bitcoin|11|CBTC|a::1|https://r",
		"admin is no party": "CBTC|Bitcoin|8|CBTC|cbtc-network|https://r",
		"registry no url":   "CBTC|Bitcoin|8|CBTC|a::1|registry",
		"symbol twice":      "CBTC|Bitcoin|8|CBTC|a::1|https://r,CBTC|Other|8|X|a::1|https://r",
		"instrument twice":  "CBTC|Bitcoin|8|CBTC|a::1|https://r,XBT|Other|8|CBTC|a::1|https://r",
	} {
		t.Run(name, func(t *testing.T) {
			var list domain.InstrumentList
			require.ErrorIs(t, list.UnmarshalText([]byte(text)), domain.ErrValidation)
		})
	}
}

func Test_Network_Parse(t *testing.T) {
	var n domain.Network

	require.NoError(t, n.UnmarshalText([]byte("mainnet")))
	require.Equal(t, domain.NetworkMainNet, n)
	require.ErrorIs(t, n.UnmarshalText([]byte("sandbox")), domain.ErrValidation)
}
