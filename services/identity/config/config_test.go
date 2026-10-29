package config_test

import (
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/services/identity/config"
)

func leftUnset(t *testing.T, key string) {
	t.Helper()

	t.Setenv(key, "")
	require.NoError(t, os.Unsetenv(key))
}

type envVar struct {
	value string
	set   bool
}

func environment(keys []string) map[string]envVar {
	state := make(map[string]envVar, len(keys))

	for _, key := range keys {
		value, set := os.LookupEnv(key)
		state[key] = envVar{value: value, set: set}
	}

	return state
}

func setRequired(t *testing.T) {
	t.Helper()

	t.Setenv("POSTGRES_USER", "otc")
	t.Setenv("POSTGRES_PASSWORD", "otc")
	t.Setenv("POSTGRES_PORT", "5432")
	t.Setenv("POSTGRES_HOST", "localhost")
	t.Setenv("POSTGRES_DB_NAME", "identity")
	t.Setenv("REDIS_ADDR", "localhost:6379")
}

func Test_New_Defaults(t *testing.T) {
	t.Chdir(t.TempDir())
	setRequired(t)

	c, err := config.New()
	require.NoError(t, err)

	require.Equal(t, "8081", c.HTTP.Port)
	require.Equal(t, "9081", c.GRPC.Port)
	require.Equal(t, "identity", c.OTEL.ServiceName)
	require.Equal(t, "localhost:9083", c.Ledger.Addr)
	require.Equal(t, 168*time.Hour, c.UseCase.SessionTTL)
	require.False(t, c.Router.CookieSecure)
	require.Empty(t, c.UseCase.MakerParty)
	require.Empty(t, c.UseCase.MakerAPIKey)
}

func Test_New_Overrides(t *testing.T) {
	t.Chdir(t.TempDir())
	setRequired(t)
	t.Setenv("HTTP_PORT", "9090")
	t.Setenv("GRPC_PORT", "9091")
	t.Setenv("OTEL_SERVICE_NAME", "identity-2")
	t.Setenv("LEDGER_GRPC_ADDR", "ledger:9083")
	t.Setenv("SESSION_TTL", "2h")
	t.Setenv("COOKIE_SECURE", "true")
	t.Setenv("MAKER_PARTY", "bot::1220ab")
	t.Setenv("MAKER_API_KEY", "secret")

	c, err := config.New()
	require.NoError(t, err)

	require.Equal(t, "9090", c.HTTP.Port)
	require.Equal(t, "9091", c.GRPC.Port)
	require.Equal(t, "identity-2", c.OTEL.ServiceName)
	require.Equal(t, "ledger:9083", c.Ledger.Addr)
	require.Equal(t, 2*time.Hour, c.UseCase.SessionTTL)
	require.True(t, c.Router.CookieSecure)
	require.Equal(t, "bot::1220ab", c.UseCase.MakerParty)
	require.Equal(t, "secret", c.UseCase.MakerAPIKey)
}

func Test_New_ReadsDotEnv(t *testing.T) {
	dotenv := map[string]string{"HTTP_PORT": "7000", "SESSION_TTL": "3h"}
	keys := slices.Sorted(maps.Keys(dotenv))
	before := environment(keys)

	t.Run("the file is read", func(t *testing.T) {
		t.Chdir(t.TempDir())
		setRequired(t)

		var file strings.Builder

		for _, key := range keys {
			leftUnset(t, key)
			fmt.Fprintf(&file, "%s=%s\n", key, dotenv[key])
		}

		require.NoError(t, os.WriteFile(".env", []byte(file.String()), 0o600))

		c, err := config.New()
		require.NoError(t, err)

		require.Equal(t, "7000", c.HTTP.Port)
		require.Equal(t, 3*time.Hour, c.UseCase.SessionTTL)
	})

	require.Equal(t, before, environment(keys), "loading a .env file must leave the process environment as it was")
}

func Test_New_MissingRequired(t *testing.T) {
	t.Chdir(t.TempDir())
	setRequired(t)
	require.NoError(t, os.Unsetenv("REDIS_ADDR"))

	_, err := config.New()
	require.ErrorContains(t, err, "REDIS_ADDR")
}

func Test_New_Rejects(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		want []string
	}{
		{
			name: "maker without a key",
			env:  map[string]string{"MAKER_PARTY": "maker::1220ab"},
			want: []string{"MAKER_PARTY and MAKER_API_KEY go together"},
		},
		{
			name: "maker by hint",
			env:  map[string]string{"MAKER_PARTY": "maker", "MAKER_API_KEY": "k"},
			want: []string{"MAKER_PARTY must be a full party id"},
		},
		{
			name: "session shorter than 1m",
			env:  map[string]string{"SESSION_TTL": "30s"},
			want: []string{"SESSION_TTL must be at least 1m"},
		},
		{
			name: "session just under 1m",
			env:  map[string]string{"SESSION_TTL": "59s"},
			want: []string{"SESSION_TTL must be at least 1m"},
		},
		{
			name: "session of zero",
			env:  map[string]string{"SESSION_TTL": "0s"},
			want: []string{"SESSION_TTL must be at least 1m"},
		},
		{
			name: "negative session",
			env:  map[string]string{"SESSION_TTL": "-1h"},
			want: []string{"SESSION_TTL must be at least 1m"},
		},
		{
			name: "session that is no time",
			env:  map[string]string{"SESSION_TTL": "a week"},
			want: []string{`field "SessionTTL"`, "invalid duration"},
		},
		{
			name: "cookie flag that is no bool",
			env:  map[string]string{"COOKIE_SECURE": "maybe"},
			want: []string{`field "CookieSecure"`, "invalid syntax"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			setRequired(t)

			for key, value := range c.env {
				t.Setenv(key, value)
			}

			_, err := config.New()
			require.Error(t, err)

			for _, want := range c.want {
				require.ErrorContains(t, err, want)
			}
		})
	}
}

func Test_New_AcceptsASessionOfExactlyOneMinute(t *testing.T) {
	t.Chdir(t.TempDir())
	setRequired(t)
	t.Setenv("SESSION_TTL", "1m")

	c, err := config.New()
	require.NoError(t, err)
	require.Equal(t, time.Minute, c.UseCase.SessionTTL)
}
