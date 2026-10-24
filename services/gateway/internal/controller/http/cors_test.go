package http_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/pkg/principal"
	"github.com/vbloher/canton-private-otc-desk/pkg/router"
	controller "github.com/vbloher/canton-private-otc-desk/services/gateway/internal/controller/http"
	"github.com/vbloher/canton-private-otc-desk/services/gateway/internal/controller/ws"
	"github.com/vbloher/canton-private-otc-desk/services/gateway/internal/usecase"
	"github.com/vbloher/canton-private-otc-desk/services/gateway/internal/usecase/mocks"
)

// pagesOrigin is where the web app is hosted when it is not served by the gateway's own origin.
const pagesOrigin = "https://vbloher.github.io"

func withCORS(origins ...string) func(*controller.Config) {
	return func(c *controller.Config) { c.CORSOrigins = origins }
}

func fromOrigin(origin string) func(*http.Request) {
	return withHeader("Origin", origin)
}

func preflight(origin string) []func(*http.Request) {
	return []func(*http.Request){
		fromOrigin(origin),
		withHeader("Access-Control-Request-Method", http.MethodPost),
		withHeader("Access-Control-Request-Headers", "authorization,content-type"),
	}
}

// requireCORS: the response lets the page at origin read it, without credentials.
func requireCORS(t *testing.T, rec *httptest.ResponseRecorder, origin string, msg ...any) {
	t.Helper()

	require.Equal(t, []string{origin}, rec.Header().Values("Access-Control-Allow-Origin"), msg...)
	require.Contains(t, rec.Header().Values("Vary"), "Origin", msg...)
	require.Empty(t, rec.Header().Values("Access-Control-Allow-Credentials"), msg...)
}

// requireNoCORS: the response is exactly what a gateway without CORS would send.
func requireNoCORS(t *testing.T, rec *httptest.ResponseRecorder, msg ...any) {
	t.Helper()

	for name := range rec.Header() {
		require.False(t, strings.HasPrefix(strings.ToLower(name), "access-control-"), append([]any{name}, msg...)...)
	}

	require.Empty(t, rec.Header().Values("Vary"), msg...)
}

func Test_CORS_Preflight(t *testing.T) {
	// A strict identity mock: a preflight must be answered before authentication is even attempted.
	g := newGateway(t, mocks.NewIdentity(t), withCORS(pagesOrigin))

	paths := []string{
		"/api/v1/rfqs", "/api/v1/rfqs/9f1c/quotes", "/api/v1/wallet/withdraw", "/api/v1/me", "/api/v1/auth/login",
		"/api/v1/ws", "/api/v1/unknown", "/not/the/api", "/metrics",
	}

	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			rec := do(t, g, http.MethodOptions, path, "", preflight(pagesOrigin)...)

			require.Equal(t, http.StatusNoContent, rec.Code)
			require.Empty(t, rec.Body.String())
			requireCORS(t, rec, pagesOrigin)
			require.Equal(t, "GET, POST, PUT, DELETE", rec.Header().Get("Access-Control-Allow-Methods"))
			require.Equal(t, "Authorization, Content-Type, X-API-Key", rec.Header().Get("Access-Control-Allow-Headers"))
			require.Equal(t, "600", rec.Header().Get("Access-Control-Max-Age"))
		})
	}

	require.Zero(t, g.totalHits(), "a preflight never reaches a service")
}

// The headers are on whatever the browser is about to read, errors included.
func Test_CORS_AllowedOriginOnEveryResponse(t *testing.T) {
	idp := identityKnowsAlice(t)
	idp.EXPECT().Authenticate(Any, "outage").Return(principal.Principal{}, io.ErrUnexpectedEOF)

	tests := map[string]struct {
		method, path string
		mutate       []func(*http.Request)
		arrange      func(*gateway)
		status       int
	}{
		"proxied success": {http.MethodGet, "/api/v1/rfqs/mine", []func(*http.Request){withBearer("alice-session")}, nil, http.StatusOK},
		"public route":    {http.MethodPost, "/api/v1/auth/login", nil, nil, http.StatusOK},
		"401 from the gateway": {
			http.MethodGet, "/api/v1/me", nil, nil, http.StatusUnauthorized,
		},
		"404 from the gateway":    {http.MethodGet, "/api/v1/unknown", nil, nil, http.StatusNotFound},
		"probe":                   {http.MethodGet, "/live", nil, nil, http.StatusOK},
		"502, service down":       {http.MethodGet, "/api/v1/trades", []func(*http.Request){withBearer("alice-session")}, func(g *gateway) { g.trading.Close() }, http.StatusBadGateway},
		"502, public route down":  {http.MethodPost, "/api/v1/auth/login", nil, func(g *gateway) { g.identity.Close() }, http.StatusBadGateway},
		"502, identity unreached": {http.MethodGet, "/api/v1/me", []func(*http.Request){withBearer("outage")}, nil, http.StatusBadGateway},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			g := newGateway(t, idp, withCORS(pagesOrigin))
			if tt.arrange != nil {
				tt.arrange(g)
			}

			rec := do(t, g, tt.method, tt.path, "", append(tt.mutate, fromOrigin(pagesOrigin))...)

			require.Equal(t, tt.status, rec.Code)
			requireCORS(t, rec, pagesOrigin)
			require.Empty(t, rec.Header().Values("Access-Control-Allow-Methods"), "only a preflight lists methods")
		})
	}
}

func Test_CORS_ErrorFromAServiceKeepsItsHeadersAndGainsOurs(t *testing.T) {
	trading := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Vary", "Accept-Encoding")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_, _ = io.WriteString(w, `{"error":"quote is not open","code":"QUOTE_NOT_OPEN"}`)
	}))
	t.Cleanup(trading.Close)

	h, _ := newRouter(t, identityKnowsAlice(t), controller.Config{
		IdentityURL: "http://127.0.0.1:1", TradingURL: trading.URL, LedgerURL: "http://127.0.0.1:1",
		CORSOrigins: []string{pagesOrigin},
	})

	rec := do(t, h, http.MethodPost, "/api/v1/quotes/7a2b/accept", "", withBearer("alice-session"), fromOrigin(pagesOrigin))

	require.Equal(t, http.StatusConflict, rec.Code)
	require.JSONEq(t, `{"error":"quote is not open","code":"QUOTE_NOT_OPEN"}`, rec.Body.String())
	requireCORS(t, rec, pagesOrigin)
	require.Contains(t, rec.Header().Values("Vary"), "Accept-Encoding", "the service's own Vary stays")
}

func Test_CORS_OtherOriginsGetNothing(t *testing.T) {
	origins := map[string]string{
		"unrelated site":      "https://evil.example",
		"suffix trick":        "https://vbloher.github.io.evil.example",
		"prefix trick":        "https://evil-vbloher.github.io",
		"another scheme":      "http://vbloher.github.io",
		"another port":        "https://vbloher.github.io:8443",
		"subdomain":           "https://app.vbloher.github.io",
		"opaque origin":       "null",
		"wildcard":            "*",
		"with a path":         "https://vbloher.github.io/canton-private-otc-desk",
		"configured, wrongly": "https://vbloher.github.io/",
	}

	for name, origin := range origins {
		t.Run(name, func(t *testing.T) {
			g := newGateway(t, identityKnowsAlice(t), withCORS(pagesOrigin))

			rec := do(t, g, http.MethodGet, "/api/v1/rfqs/mine", "", withBearer("alice-session"), fromOrigin(origin))
			require.Equal(t, http.StatusOK, rec.Code, "the request itself is served as ever")
			requireNoCORS(t, rec)

			rec = do(t, g, http.MethodGet, "/api/v1/me", "", fromOrigin(origin))
			require.Equal(t, http.StatusUnauthorized, rec.Code)
			requireNoCORS(t, rec)

			// Its preflight is not ours to answer: it goes down the usual road, which says 401.
			rec = do(t, g, http.MethodOptions, "/api/v1/rfqs", "", preflight(origin)...)
			require.Equal(t, http.StatusUnauthorized, rec.Code)
			requireNoCORS(t, rec)
		})
	}
}

func Test_CORS_NoOriginNoHeaders(t *testing.T) {
	g := newGateway(t, identityKnowsAlice(t), withCORS(pagesOrigin))

	rec := do(t, g, http.MethodGet, "/api/v1/rfqs/mine", "", withBearer("alice-session"))

	require.Equal(t, http.StatusOK, rec.Code)
	requireNoCORS(t, rec)
}

func Test_CORS_OffByDefault(t *testing.T) {
	for name, options := range map[string][]func(*controller.Config){
		"not configured": nil,
		"blank entries":  {withCORS("", "  ")},
	} {
		t.Run(name, func(t *testing.T) {
			g := newGateway(t, identityKnowsAlice(t), options...)

			rec := do(t, g, http.MethodGet, "/api/v1/rfqs/mine", "", withBearer("alice-session"), fromOrigin(pagesOrigin))
			require.Equal(t, http.StatusOK, rec.Code)
			requireNoCORS(t, rec)

			rec = do(t, g, http.MethodOptions, "/api/v1/rfqs", "", preflight(pagesOrigin)...)
			require.Equal(t, http.StatusUnauthorized, rec.Code, "without CORS an OPTIONS request is an ordinary one")
			requireNoCORS(t, rec)
		})
	}
}

func Test_CORS_OptionsWithoutRequestMethodIsNotAPreflight(t *testing.T) {
	g := newGateway(t, mocks.NewIdentity(t), withCORS(pagesOrigin))

	rec := do(t, g, http.MethodOptions, "/api/v1/rfqs", "", fromOrigin(pagesOrigin))

	require.Equal(t, http.StatusUnauthorized, rec.Code, "it is routed and authenticated like any request")
	requireCORS(t, rec, pagesOrigin)
	require.Empty(t, rec.Header().Values("Access-Control-Allow-Methods"))
	require.Empty(t, rec.Header().Values("Access-Control-Max-Age"))
}

func Test_CORS_EchoesTheCallersOriginOnly(t *testing.T) {
	g := newGateway(t, identityKnowsAlice(t),
		withCORS(pagesOrigin, "http://localhost:5173", "https://desk.example.com:8443"))

	for _, origin := range []string{pagesOrigin, "http://localhost:5173", "https://desk.example.com:8443"} {
		rec := do(t, g, http.MethodGet, "/api/v1/rfqs/mine", "", withBearer("alice-session"), fromOrigin(origin))

		require.Equal(t, http.StatusOK, rec.Code)
		requireCORS(t, rec, origin, "never the whole list, never *")
	}
}

// Browsers send the origin lower-case and without a default port; the configuration may be written otherwise.
func Test_CORS_NormalisesTheConfiguredOrigins(t *testing.T) {
	g := newGateway(t, identityKnowsAlice(t),
		withCORS("HTTPS://VBloher.GitHub.io", "  http://localhost:5173 ", "https://desk.example.com:443", "http://127.0.0.1:80", ""))

	for _, origin := range []string{"https://vbloher.github.io", "http://localhost:5173", "https://desk.example.com", "http://127.0.0.1"} {
		rec := do(t, g, http.MethodGet, "/api/v1/rfqs/mine", "", withBearer("alice-session"), fromOrigin(origin))

		require.Equal(t, http.StatusOK, rec.Code)
		requireCORS(t, rec, origin)
	}
}

func Test_Router_AcceptsValidCORSOrigins(t *testing.T) {
	valid := []string{
		"https://vbloher.github.io", "http://localhost:5173", "https://desk.example.com:8443", "http://127.0.0.1:3000",
		"http://[::1]:5173", "HTTPS://Desk.Example.COM", "https://a-b.example.com",
	}

	for _, origin := range valid {
		t.Run(origin, func(t *testing.T) {
			_, err := controller.GatewayRouter(router.New(), usecase.New(mocks.NewIdentity(t)), ws.NewHub(ws.PingInterval),
				controller.Config{
					IdentityURL: "http://identity:8081", TradingURL: "http://trading:8082", LedgerURL: "http://ledger:8083",
					CORSOrigins: []string{origin},
				}, sharedMetrics())
			require.NoError(t, err)
		})
	}
}

func Test_Router_RejectsInvalidCORSOrigin(t *testing.T) {
	invalid := map[string]string{
		"wildcard":             "*",
		"wildcard subdomain":   "https://*.github.io",
		"no scheme":            "vbloher.github.io",
		"scheme only":          "https://",
		"trailing slash":       "https://vbloher.github.io/",
		"path":                 "https://vbloher.github.io/canton-private-otc-desk",
		"query":                "https://vbloher.github.io?x=1",
		"empty query":          "https://vbloher.github.io?",
		"fragment":             "https://vbloher.github.io#top",
		"user info":            "https://user@vbloher.github.io",
		"another scheme":       "ftp://vbloher.github.io",
		"websocket scheme":     "wss://vbloher.github.io",
		"opaque":               "null",
		"port zero":            "https://vbloher.github.io:0",
		"port too large":       "https://vbloher.github.io:99999",
		"port is not a number": "https://vbloher.github.io:https",
		"empty port":           "https://vbloher.github.io:",
		"space in host":        "https://vbloher .github.io",
		"underscore in host":   "https://my_app.example.com",
		"two origins":          "https://a.example.com https://b.example.com",
		"bad ipv6":             "http://[::1",
		"ipv6 zone":            "http://[fe80::1%25en0]:5173",
	}

	for name, origin := range invalid {
		t.Run(name, func(t *testing.T) {
			_, err := controller.GatewayRouter(router.New(), usecase.New(mocks.NewIdentity(t)), ws.NewHub(ws.PingInterval),
				controller.Config{
					IdentityURL: "http://identity:8081", TradingURL: "http://trading:8082", LedgerURL: "http://ledger:8083",
					CORSOrigins: []string{"https://vbloher.github.io", origin},
				}, sharedMetrics())

			require.ErrorIs(t, err, controller.ErrInvalidOrigin)
			require.ErrorContains(t, err, origin, "the message names the entry to fix")
		})
	}
}
