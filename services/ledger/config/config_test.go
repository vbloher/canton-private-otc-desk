package config_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/services/ledger/config"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/domain"
)

const instruments = "CBTC|Bitcoin|8|CBTC|cbtc-network::1220aa|https://utilities.example/registrars/cbtc-network::1220aa/registry," +
	"CC|Canton Coin|4|Amulet|DSO::1220bb|https://scan.example/registry"

func setRequired(t *testing.T) {
	t.Helper()

	t.Setenv("POSTGRES_USER", "otc")
	t.Setenv("POSTGRES_PASSWORD", "otc")
	t.Setenv("POSTGRES_PORT", "5432")
	t.Setenv("POSTGRES_HOST", "localhost")
	t.Setenv("POSTGRES_DB_NAME", "ledger")
	t.Setenv("REDIS_ADDR", "localhost:6379")
	t.Setenv("CANTON_URL", "https://ledger-api.validator.example")
	t.Setenv("OPERATOR_PARTY", "otc-operator::1220aa")
	t.Setenv("INSTRUMENTS", instruments)
}

func Test_New_Defaults(t *testing.T) {
	t.Chdir(t.TempDir())
	setRequired(t)

	c, err := config.New()
	require.NoError(t, err)

	require.Equal(t, "8083", c.HTTP.Port)
	require.Equal(t, "9083", c.GRPC.Port)
	require.Equal(t, "ledger", c.OTEL.ServiceName)
	require.Equal(t, "https://ledger-api.validator.example", c.Canton.URL)
	require.Empty(t, c.Canton.UserID)
	require.Empty(t, c.Canton.Auth.TokenURL)
	require.Equal(t, domain.NetworkDevNet, c.Ledger.Network)
	require.Equal(t, "otc-operator::1220aa", c.Ledger.OperatorParty)
	require.Empty(t, c.Ledger.MakerParty)
	require.Empty(t, c.Ledger.FaucetParty)
	require.Empty(t, c.Ledger.FaucetAmounts)
	require.Equal(t, time.Hour, c.Ledger.FaucetCooldown)
	require.Equal(t, 10*time.Minute, c.Ledger.ActionTTL)

	require.Len(t, c.Ledger.Instruments, 2)
	require.Equal(t, "Amulet", c.Ledger.Instruments[1].ID)
	require.Equal(t, "DSO::1220bb", c.Ledger.Instruments[1].Admin)
	require.Equal(t, "https://scan.example/registry", c.Ledger.Instruments[1].Registry)
}

func Test_New_Overrides(t *testing.T) {
	t.Chdir(t.TempDir())
	setRequired(t)
	t.Setenv("HTTP_PORT", "9999")
	t.Setenv("LOGGER_LEVEL", "debug")
	t.Setenv("CANTON_NETWORK", "mainnet")
	t.Setenv("CANTON_USER_ID", "otc-desk")
	t.Setenv("CANTON_AUTH_TOKEN_URL", "https://idp.example/realms/canton/protocol/openid-connect/token")
	t.Setenv("CANTON_AUTH_CLIENT_ID", "otc-desk")
	t.Setenv("CANTON_AUTH_CLIENT_SECRET", "secret")
	t.Setenv("CANTON_AUTH_AUDIENCE", "https://validator.example")
	t.Setenv("CANTON_AUTH_SCOPE", "daml_ledger_api")
	t.Setenv("MAKER_PARTY", "otc-maker::1220aa")
	t.Setenv("FAUCET_PARTY", "otc-faucet::1220aa")
	t.Setenv("FAUCET_AMOUNTS", "CBTC:0.01,CC:100")
	t.Setenv("FAUCET_COOLDOWN", "30m")

	c, err := config.New()
	require.NoError(t, err)

	require.Equal(t, "9999", c.HTTP.Port)
	require.Equal(t, "debug", c.Logger.Level)
	require.Equal(t, domain.NetworkMainNet, c.Ledger.Network)
	require.Equal(t, "otc-desk", c.Canton.UserID)
	require.Equal(t, "otc-desk", c.Canton.Auth.ClientID)
	require.Equal(t, "secret", c.Canton.Auth.ClientSecret)
	require.Equal(t, "https://validator.example", c.Canton.Auth.Audience)
	require.Equal(t, "daml_ledger_api", c.Canton.Auth.Scope)
	require.Equal(t, "otc-maker::1220aa", c.Ledger.MakerParty)
	require.Equal(t, "otc-faucet::1220aa", c.Ledger.FaucetParty)
	require.Equal(t, "100.0", c.Ledger.FaucetAmounts["CC"].String())
	require.Equal(t, 30*time.Minute, c.Ledger.FaucetCooldown)
}

func Test_New_Rejects(t *testing.T) {
	for name, tc := range map[string]struct {
		key, value, message string
	}{
		"bad instruments": {"INSTRUMENTS", "CBTC:Bitcoin:8", "want SYMBOL|Name|decimals|instrumentId|admin|registryURL"},
		"no instruments":  {"INSTRUMENTS", "", `"INSTRUMENTS" should not be empty`},
		"no ledger":       {"CANTON_URL", "", `"CANTON_URL" should not be empty`},
		"no operator":     {"OPERATOR_PARTY", "", `"OPERATOR_PARTY" should not be empty`},
		"unknown network": {"CANTON_NETWORK", "sandbox", "want devnet, testnet or mainnet"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			setRequired(t)
			t.Setenv(tc.key, tc.value)

			_, err := config.New()
			require.ErrorContains(t, err, tc.message)
		})
	}
}
