package http_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/coder/websocket"
	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/pkg/principal"
	controller "github.com/vbloher/canton-private-otc-desk/services/gateway/internal/controller/http"
	"github.com/vbloher/canton-private-otc-desk/services/gateway/internal/domain"
)

// requireNoStore: exactly one Cache-Control line, and it says the response must not be stored anywhere.
func requireNoStore(t *testing.T, header http.Header, msg ...any) {
	t.Helper()

	require.Equal(t, []string{"no-store"}, header.Values("Cache-Control"), msg...)
}

// Responses carry sessions, balances and quotes, and a login response carries the session token itself:
// no cache, shared or private, may keep one, whoever produced it.
func Test_NoStore_OnEveryAPIResponse(t *testing.T) {
	idp := identityKnowsAlice(t)
	idp.EXPECT().Authenticate(Any, "outage").Return(principal.Principal{}, io.ErrUnexpectedEOF)

	signedIn := []func(*http.Request){withBearer("alice-session")}

	tests := map[string]struct {
		method, path string
		mutate       []func(*http.Request)
		arrange      func(*gateway)
		status       int
	}{
		"identity route":                   {http.MethodGet, "/api/v1/me", signedIn, nil, http.StatusOK},
		"trading route":                    {http.MethodGet, "/api/v1/rfqs/mine", signedIn, nil, http.StatusOK},
		"ledger route":                     {http.MethodGet, "/api/v1/wallet/balances", signedIn, nil, http.StatusOK},
		"login, which hands out the token": {http.MethodPost, "/api/v1/auth/login", nil, nil, http.StatusOK},
		"logout":                           {http.MethodPost, "/api/v1/auth/logout", signedIn, nil, http.StatusOK},
		"401 from the gateway":             {http.MethodGet, "/api/v1/me", nil, nil, http.StatusUnauthorized},
		"404 for an unknown path":          {http.MethodGet, "/api/v1/unknown", nil, nil, http.StatusNotFound},
		"404 for /api":                     {http.MethodGet, "/api", nil, nil, http.StatusNotFound},
		"404 for /api/":                    {http.MethodGet, "/api/", nil, nil, http.StatusNotFound},
		"502, service down":                {http.MethodGet, "/api/v1/trades", signedIn, func(g *gateway) { g.trading.Close() }, http.StatusBadGateway},
		"502, public route down":           {http.MethodPost, "/api/v1/auth/login", nil, func(g *gateway) { g.identity.Close() }, http.StatusBadGateway},
		"502, identity unreached":          {http.MethodGet, "/api/v1/me", []func(*http.Request){withBearer("outage")}, nil, http.StatusBadGateway},
	}

	for name, tt := range tests {
		for variant, options := range map[string][]func(*controller.Config){
			"cors off": nil,
			"cors on":  {withCORS(pagesOrigin)},
		} {
			t.Run(name+", "+variant, func(t *testing.T) {
				g := newGateway(t, idp, options...)
				if tt.arrange != nil {
					tt.arrange(g)
				}

				rec := do(t, g, tt.method, tt.path, "", tt.mutate...)

				require.Equal(t, tt.status, rec.Code)
				requireNoStore(t, rec.Header())
			})
		}
	}
}

func Test_NoStore_ReplacesAServicesOwnCachePolicy(t *testing.T) {
	for name, header := range map[string]http.Header{
		"public and cacheable": {"Cache-Control": {"public, max-age=3600"}, "Expires": {"Wed, 21 Oct 2037 07:28:00 GMT"}},
		"several lines":        {"Cache-Control": {"max-age=60", "private"}},
		"already no-store":     {"Cache-Control": {"no-store"}},
	} {
		t.Run(name, func(t *testing.T) {
			trading := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				for key, values := range header {
					for _, value := range values {
						w.Header().Add(key, value)
					}
				}

				_, _ = io.WriteString(w, `{}`)
			}))
			t.Cleanup(trading.Close)

			h, _ := newRouter(t, identityKnowsAlice(t), controller.Config{
				IdentityURL: "http://127.0.0.1:1", TradingURL: trading.URL, LedgerURL: "http://127.0.0.1:1",
			})

			rec := do(t, h, http.MethodGet, "/api/v1/trades", "", withBearer("alice-session"))

			require.Equal(t, http.StatusOK, rec.Code)
			requireNoStore(t, rec.Header(), "the gateway's policy is the only one")
		})
	}
}

func Test_NoStore_OnPreflights(t *testing.T) {
	g := newGateway(t, identityKnowsAlice(t), withCORS(pagesOrigin))

	for _, path := range []string{"/api/v1/rfqs", "/api/v1/auth/login", "/api/v1/ws", "/api/v1/unknown"} {
		rec := do(t, g, http.MethodOptions, path, "", preflight(pagesOrigin)...)

		require.Equal(t, http.StatusNoContent, rec.Code, path)
		requireNoStore(t, rec.Header(), path)
	}
}

func Test_NoStore_OnTheSocketRoute(t *testing.T) {
	g := serveSocketGateway(t, identityKnowsAlice(t))

	_, refused, err := g.dial(t, &websocket.DialOptions{})
	require.Error(t, err)
	require.Equal(t, http.StatusUnauthorized, refused.StatusCode)
	requireNoStore(t, refused.Header, "a refusal")

	_, upgraded := g.mustDial(t, &websocket.DialOptions{HTTPHeader: http.Header{domain.APIKeyHeader: []string{"alice-key"}}})
	requireNoStore(t, upgraded.Header, "the upgrade")
}

// Only the API is covered: probes and metrics are not API responses and stay as they were.
func Test_NoStore_LeavesOtherPathsAlone(t *testing.T) {
	g := newGateway(t, identityKnowsAlice(t))

	for _, path := range []string{"/live", "/ready", "/metrics", "/apiary", "/not/the/api"} {
		rec := do(t, g, http.MethodGet, path, "")

		require.Empty(t, rec.Header().Values("Cache-Control"), path)
	}
}
