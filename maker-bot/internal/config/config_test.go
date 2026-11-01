package config_test

import (
	"log/slog"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/maker-bot/internal/config"
)

func TestParse_Defaults(t *testing.T) {
	c, err := config.Parse(nil)
	require.NoError(t, err)

	require.Equal(t, "http://localhost:8080", c.DeskURL)
	require.Equal(t, "OTC Demo Maker", c.MakerName)
	require.Equal(t, int64(15), c.SpreadBps)
	require.Equal(t, 600*time.Second, c.QuoteTTL)
	require.InDelta(t, 0.5, c.MaxSpendShare, 0)
	require.Equal(t, 10*time.Second, c.PollInterval)
	require.Equal(t, "state/identity.json", c.StateFile)
	require.Equal(t, slog.LevelInfo, c.LogLevel)
	require.Len(t, c.Prices, 3)
	require.Zero(t, c.Prices["CBTC"].Cmp(big.NewRat(100000, 1)))
	require.Zero(t, c.Prices["USDCx"].Cmp(big.NewRat(1, 1)), "symbols keep their case")
	require.Zero(t, c.Prices["CC"].Cmp(big.NewRat(15, 100)))
}

func TestParse_Overrides(t *testing.T) {
	c, err := config.Parse(map[string]string{
		"DESK_URL":        "https://otc.vbloher.org/",
		"MAKER_NAME":      "  Night Maker  ",
		"SPREAD_BPS":      "0",
		"PRICES_USD":      "CBTC:97000.5",
		"QUOTE_TTL":       "10s",
		"MAX_SPEND_SHARE": "1",
		"POLL_INTERVAL":   "1s",
		"STATE_FILE":      "/data/identity.json",
		"LOG_LEVEL":       "debug",
	})
	require.NoError(t, err)

	require.Equal(t, "https://otc.vbloher.org", c.DeskURL)
	require.Equal(t, "Night Maker", c.MakerName)
	require.Equal(t, int64(0), c.SpreadBps)
	require.Zero(t, c.Prices["CBTC"].Cmp(big.NewRat(194001, 2)))
	require.Equal(t, 10*time.Second, c.QuoteTTL)
	require.InDelta(t, 1.0, c.MaxSpendShare, 0)
	require.Equal(t, time.Second, c.PollInterval)
	require.Equal(t, "/data/identity.json", c.StateFile)
	require.Equal(t, slog.LevelDebug, c.LogLevel)
}

func TestParse_Rejects(t *testing.T) {
	for _, environ := range []map[string]string{
		{"DESK_URL": "ftp://otc.vbloher.org"},
		{"MAKER_NAME": "a"},
		{"MAKER_NAME": "a name that is far too long for the desk"},
		{"SPREAD_BPS": "10000"},
		{"SPREAD_BPS": "-1"},
		{"SPREAD_BPS": "wide"},
		{"QUOTE_TTL": "5s"},
		{"QUOTE_TTL": "601s"},
		{"MAX_SPEND_SHARE": "0"},
		{"MAX_SPEND_SHARE": "1.5"},
		{"POLL_INTERVAL": "500ms"},
		{"PRICES_USD": "CBTC:0"},
		{"PRICES_USD": "CBTC:1,CBTC:2"},
		{"PRICES_USD": "CBTC"},
		{"PRICES_USD": "CBTC:cheap"},
		{"LOG_LEVEL": "loud"},
	} {
		_, err := config.Parse(environ)
		require.ErrorIs(t, err, config.ErrInvalid, "%v", environ)
	}
}
