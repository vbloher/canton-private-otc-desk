package config_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/services/trading/config"
)

func setRequired(t *testing.T) {
	t.Helper()

	t.Chdir(t.TempDir())
	t.Setenv("POSTGRES_USER", "otc")
	t.Setenv("POSTGRES_PASSWORD", "otc")
	t.Setenv("POSTGRES_PORT", "5432")
	t.Setenv("POSTGRES_HOST", "localhost")
	t.Setenv("POSTGRES_DB_NAME", "trading")
	t.Setenv("REDIS_ADDR", "localhost:6379")
}

func Test_New_Defaults(t *testing.T) {
	setRequired(t)

	c, err := config.New()
	require.NoError(t, err)
	require.Equal(t, "8082", c.HTTP.Port)
	require.Equal(t, "trading", c.OTEL.ServiceName)
	require.Equal(t, "localhost:9083", c.Ledger.Addr)
	require.Equal(t, "localhost:9081", c.Identity.Addr)
	require.Equal(t, int64(10), c.UseCase.FeeBps)
	require.Equal(t, 10*time.Minute, c.UseCase.SettleTimeout)
	require.Equal(t, time.Second, c.Worker.Interval)
}

func Test_New_FeeBpsBounds(t *testing.T) {
	for _, fee := range []string{"0", "9999"} {
		t.Run(fee, func(t *testing.T) {
			setRequired(t)
			t.Setenv("FEE_BPS", fee)

			_, err := config.New()
			require.NoError(t, err)
		})
	}

	for _, fee := range []string{"-1", "10000", "25000"} {
		t.Run(fee, func(t *testing.T) {
			setRequired(t)
			t.Setenv("FEE_BPS", fee)

			_, err := config.New()
			require.ErrorContains(t, err, "FEE_BPS must be between 0 and 9999, got "+fee)
		})
	}
}

func Test_New_RejectsANonPositiveSettleTimeout(t *testing.T) {
	setRequired(t)
	t.Setenv("SETTLE_TIMEOUT", "0s")

	_, err := config.New()
	require.ErrorContains(t, err, "SETTLE_TIMEOUT must be positive")
}
