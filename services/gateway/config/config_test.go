package config_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/services/gateway/config"
)

func isolate(t *testing.T) {
	t.Helper()

	t.Chdir(t.TempDir())

	for _, name := range []string{
		"HTTP_PORT", "IDENTITY_GRPC_ADDR", "IDENTITY_URL", "TRADING_URL", "LEDGER_URL", "OTEL_SERVICE_NAME", "REDIS_PASSWORD", "REDIS_DB",
	} {
		t.Setenv(name, "")
		require.NoError(t, os.Unsetenv(name))
	}

	t.Setenv("REDIS_ADDR", "localhost:6379")
}

func Test_New_Defaults(t *testing.T) {
	isolate(t)

	c, err := config.New()
	require.NoError(t, err)

	require.Equal(t, "8080", c.HTTP.Port)
	require.Equal(t, "gateway", c.OTEL.ServiceName)
	require.Equal(t, "localhost:6379", c.Redis.Addr)
	require.Equal(t, "localhost:9081", c.Identity.Addr)
	require.Equal(t, "http://localhost:8081", c.Upstreams.IdentityURL)
	require.Equal(t, "http://localhost:8082", c.Upstreams.TradingURL)
	require.Equal(t, "http://localhost:8083", c.Upstreams.LedgerURL)
}

func Test_New_Overrides(t *testing.T) {
	isolate(t)

	t.Setenv("HTTP_PORT", "9090")
	t.Setenv("OTEL_SERVICE_NAME", "edge")
	t.Setenv("IDENTITY_GRPC_ADDR", "identity:9081")
	t.Setenv("IDENTITY_URL", "http://identity:8081")
	t.Setenv("TRADING_URL", "http://trading:8082")
	t.Setenv("LEDGER_URL", "http://ledger:8083")

	c, err := config.New()
	require.NoError(t, err)

	require.Equal(t, "9090", c.HTTP.Port)
	require.Equal(t, "edge", c.OTEL.ServiceName)
	require.Equal(t, "identity:9081", c.Identity.Addr)
	require.Equal(t, "http://identity:8081", c.Upstreams.IdentityURL)
	require.Equal(t, "http://trading:8082", c.Upstreams.TradingURL)
	require.Equal(t, "http://ledger:8083", c.Upstreams.LedgerURL)
}

func Test_New_ReadsDotEnv(t *testing.T) {
	isolate(t)

	require.NoError(t, os.WriteFile(".env", []byte("HTTP_PORT=7070\nTRADING_URL=http://from-dotenv:1\n"), 0o600))

	c, err := config.New()
	require.NoError(t, err)

	require.Equal(t, "7070", c.HTTP.Port)
	require.Equal(t, "http://from-dotenv:1", c.Upstreams.TradingURL)
}

func Test_New_MissingRedis(t *testing.T) {
	isolate(t)

	require.NoError(t, os.Unsetenv("REDIS_ADDR"))

	_, err := config.New()
	require.ErrorContains(t, err, "REDIS_ADDR")
}
