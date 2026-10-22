//go:build e2e

package e2e

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	identity "github.com/vbloher/canton-private-otc-desk/gen/http/identity_v1/client"
	trading "github.com/vbloher/canton-private-otc-desk/gen/http/trading_v1/client"
	wallet "github.com/vbloher/canton-private-otc-desk/gen/http/wallet_v1/client"
)

const (
	waitFor = 60 * time.Second
	tick    = 500 * time.Millisecond
)

func baseURL() string {
	if u := os.Getenv("E2E_GATEWAY_URL"); u != "" {
		return u
	}

	return "http://localhost:8080/api/v1"
}

// user is one caller of the public API: a browser wallet (cookie + browser-held key) or a bot (API key).
type user struct {
	identity *identity.ClientWithResponses
	trading  *trading.ClientWithResponses
	wallet   *wallet.ClientWithResponses
	key      ed25519.PrivateKey
	me       identity.Me
}

func newUser(t *testing.T, apiKey string) *user {
	t.Helper()

	jar, err := cookiejar.New(nil)
	require.NoError(t, err)

	httpClient := &http.Client{Jar: jar, Timeout: 90 * time.Second}

	addKey := func(_ context.Context, req *http.Request) error {
		if apiKey != "" {
			req.Header.Set("X-API-Key", apiKey)
		}

		return nil
	}

	u := &user{}

	u.identity, err = identity.NewClientWithResponses(baseURL(),
		identity.WithHTTPClient(httpClient), identity.WithRequestEditorFn(addKey))
	require.NoError(t, err)

	u.trading, err = trading.NewClientWithResponses(baseURL(),
		trading.WithHTTPClient(httpClient), trading.WithRequestEditorFn(addKey))
	require.NoError(t, err)

	u.wallet, err = wallet.NewClientWithResponses(baseURL(),
		wallet.WithHTTPClient(httpClient), wallet.WithRequestEditorFn(addKey))
	require.NoError(t, err)

	return u
}

// newWallet creates a passkey-style wallet the way the browser does, with the key held here.
func newWallet(t *testing.T, displayName string) *user {
	t.Helper()

	u := newUser(t, "")

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	u.key = priv

	start, err := u.identity.RegisterStartWithResponse(t.Context(), identity.RegisterStartInput{
		DisplayName: displayName,
		PublicKey:   base64.StdEncoding.EncodeToString(pub),
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, start.StatusCode(), string(start.Body))

	complete, err := u.identity.RegisterCompleteWithResponse(t.Context(), identity.RegisterCompleteInput{
		RegistrationID: start.JSON200.RegistrationID,
		Signature:      u.sign(t, start.JSON200.MultiHash),
		CredentialID:   "e2e-" + uuid.NewString(),
		EncryptedBlob:  "e2e",
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, complete.StatusCode(), string(complete.Body))

	u.me = *complete.JSON200
	require.Equal(t, start.JSON200.PartyID, u.me.PartyID)

	return u
}

func (u *user) sign(t *testing.T, hashB64 string) string {
	t.Helper()

	hash, err := base64.StdEncoding.DecodeString(hashB64)
	require.NoError(t, err)

	return base64.StdEncoding.EncodeToString(ed25519.Sign(u.key, hash))
}

// signPending signs and executes every action waiting for this wallet; it returns how many it signed.
func (u *user) signPending(t *testing.T) int {
	t.Helper()

	signed, err := u.trySignPending(t.Context())
	require.NoError(t, err)

	return signed
}

// trySignPending is signPending for use inside Eventually closures: it reports instead of failing.
func (u *user) trySignPending(ctx context.Context) (int, error) {
	pending, err := u.wallet.ListPendingActionsWithResponse(ctx)
	if err != nil {
		return 0, fmt.Errorf("list pending: %w", err)
	}

	if pending.StatusCode() != http.StatusOK {
		return 0, fmt.Errorf("list pending: %d %s", pending.StatusCode(), pending.Body)
	}

	for _, action := range *pending.JSON200 {
		hash, err := base64.StdEncoding.DecodeString(action.Hash)
		if err != nil {
			return 0, fmt.Errorf("decode hash: %w", err)
		}

		executed, err := u.wallet.ExecuteActionWithResponse(ctx, action.ID,
			wallet.ExecuteInput{Signature: base64.StdEncoding.EncodeToString(ed25519.Sign(u.key, hash))})
		if err != nil {
			return 0, fmt.Errorf("execute: %w", err)
		}

		if executed.StatusCode() != http.StatusNoContent {
			return 0, fmt.Errorf("execute: %d %s", executed.StatusCode(), executed.Body)
		}
	}

	return len(*pending.JSON200), nil
}

func (u *user) balance(t *testing.T, symbol string) wallet.Balance {
	t.Helper()

	b, err := u.tryBalance(t.Context(), symbol)
	require.NoError(t, err)

	return b
}

// tryBalance is balance for use inside Eventually closures: it reports instead of failing.
func (u *user) tryBalance(ctx context.Context, symbol string) (wallet.Balance, error) {
	balances, err := u.wallet.GetBalancesWithResponse(ctx)
	if err != nil {
		return wallet.Balance{}, fmt.Errorf("balances: %w", err)
	}

	if balances.StatusCode() != http.StatusOK {
		return wallet.Balance{}, fmt.Errorf("balances: %d %s", balances.StatusCode(), balances.Body)
	}

	for _, b := range *balances.JSON200 {
		if b.Symbol == symbol {
			return b, nil
		}
	}

	return wallet.Balance{}, fmt.Errorf("no balance for %s", symbol)
}

// fund mints test tokens to the wallet and signs the accepts.
func (u *user) fund(t *testing.T) {
	t.Helper()

	faucet, err := u.wallet.FaucetWithResponse(t.Context())
	require.NoError(t, err)
	require.Equal(t, http.StatusAccepted, faucet.StatusCode(), string(faucet.Body))
	require.NotEmpty(t, faucet.JSON202.Actions)

	for _, action := range faucet.JSON202.Actions {
		executed, err := u.wallet.ExecuteActionWithResponse(t.Context(), action.ID,
			wallet.ExecuteInput{Signature: u.sign(t, action.Hash)})
		require.NoError(t, err)
		require.Equal(t, http.StatusNoContent, executed.StatusCode(), string(executed.Body))
	}

	require.EventuallyWithT(t, func(c *assert.CollectT) {
		b, err := u.tryBalance(t.Context(), "USDCx")
		assert.NoError(c, err)
		assert.Equal(c, "100000.0", b.Available)
	}, waitFor, tick, "faucet USDCx never arrived")
}
