package config_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/services/ledger/config"
)

func setRequired(t *testing.T) {
	t.Helper()

	t.Setenv("POSTGRES_USER", "otc")
	t.Setenv("POSTGRES_PASSWORD", "otc")
	t.Setenv("POSTGRES_PORT", "5432")
	t.Setenv("POSTGRES_HOST", "localhost")
	t.Setenv("POSTGRES_DB_NAME", "ledger")
	t.Setenv("REDIS_ADDR", "localhost:6379")
}

func Test_New_Defaults(t *testing.T) {
	t.Chdir(t.TempDir())
	setRequired(t)

	c, err := config.New()
	require.NoError(t, err)

	require.Equal(t, "8083", c.HTTP.Port)
	require.Equal(t, "9083", c.GRPC.Port)
	require.Equal(t, "ledger", c.OTEL.ServiceName)
	require.Equal(t, "http://localhost:6864", c.Canton.URL)
	require.Equal(t, "participant_admin", c.Canton.UserID)
	require.Equal(t, "operator", c.Ledger.OperatorHint)
	require.Equal(t, []string{"makerA", "makerB"}, c.Ledger.HouseParties)
	require.Equal(t, 10*time.Minute, c.Ledger.ActionTTL)

	require.Len(t, c.Ledger.Instruments, 3)
	require.Equal(t, "USD Coin", c.Ledger.Instruments[1].Name)
	require.Equal(t, int32(2), c.Ledger.Instruments[1].Decimals)
	require.Equal(t, "100000.0", c.Ledger.FaucetAmounts["USDCx"].String())
	require.Equal(t, "1.0", c.Ledger.FaucetAmounts["CBTC"].String())
	require.Equal(t, "10.0", c.Ledger.FaucetAmounts["CETH"].String())
}

func Test_New_Overrides(t *testing.T) {
	t.Chdir(t.TempDir())
	setRequired(t)
	t.Setenv("HTTP_PORT", "9999")
	t.Setenv("LOGGER_LEVEL", "debug")
	t.Setenv("INSTRUMENTS", "CBTC:Bitcoin:8")
	t.Setenv("FAUCET_AMOUNTS", "CBTC:0.5")

	c, err := config.New()
	require.NoError(t, err)

	require.Equal(t, "9999", c.HTTP.Port)
	require.Equal(t, "debug", c.Logger.Level)
	require.Len(t, c.Ledger.Instruments, 1)
	require.Equal(t, "0.5", c.Ledger.FaucetAmounts["CBTC"].String())
}

func Test_New_RejectsBadInstruments(t *testing.T) {
	t.Chdir(t.TempDir())
	setRequired(t)
	t.Setenv("INSTRUMENTS", "CBTC:Bitcoin")

	_, err := config.New()
	require.ErrorContains(t, err, "want SYMBOL:Name:decimals")
}
