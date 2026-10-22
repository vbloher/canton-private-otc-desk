package usecase_test

import (
	"crypto/ed25519"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/usecase"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/usecase/mocks"
)

const Any = mock.Anything

var testConfig = usecase.Config{
	SessionTTL: 24 * time.Hour,
	HouseMakers: domain.HouseMakers{
		{Hint: "makerA", APIKey: "demo-key-a"},
		{Hint: "makerB", APIKey: "demo-key-b"},
	},
}

// deps holds a strict mock of every port: a call nobody expected fails the test.
type deps struct {
	postgres *mocks.Postgres
	redis    *mocks.Redis
	ledger   *mocks.Ledger
}

func newDeps(t *testing.T) deps {
	t.Helper()

	otel.SilentModeInit()

	return deps{
		postgres: mocks.NewPostgres(t),
		redis:    mocks.NewRedis(t),
		ledger:   mocks.NewLedger(t),
	}
}

func (d deps) useCase() *usecase.UseCase {
	return usecase.New(d.postgres, d.redis, d.ledger, testConfig)
}

// walletUser is a registered passkey wallet together with the private key of its browser.
func walletUser(t *testing.T) (domain.User, ed25519.PrivateKey) {
	t.Helper()

	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	user, err := domain.NewPasskeyUser(
		"alice-smith-3f9a1c::1220ab", "alice-smith-3f9a1c", "Alice Smith", publicKey, "cred-1", "blob-1",
	)
	require.NoError(t, err)

	return user, privateKey
}
