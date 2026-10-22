package http_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/pkg/metrics"
	"github.com/vbloher/canton-private-otc-desk/pkg/otel"
	"github.com/vbloher/canton-private-otc-desk/pkg/principal"
	"github.com/vbloher/canton-private-otc-desk/pkg/router"
	controller "github.com/vbloher/canton-private-otc-desk/services/gateway/internal/controller/http"
	"github.com/vbloher/canton-private-otc-desk/services/gateway/internal/controller/ws"
	"github.com/vbloher/canton-private-otc-desk/services/gateway/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/gateway/internal/usecase"
	"github.com/vbloher/canton-private-otc-desk/services/gateway/internal/usecase/mocks"
)

const Any = mock.Anything

const (
	identityName = "identity"
	tradingName  = "trading"
	ledgerName   = "ledger"
)

func TestMain(m *testing.M) {
	otel.SilentModeInit()

	m.Run()
}

// The Prometheus collectors are registered globally, so the whole test binary shares one set.
var sharedMetrics = sync.OnceValue(metrics.NewHTTPServer)

var alice = principal.Principal{UserID: uuid.New(), PartyID: "alice-3f9a1c::1220ab", Hint: "alice-3f9a1c"}

// seen is what an echo upstream received.
type seen struct {
	Upstream string      `json:"upstream"`
	Method   string      `json:"method"`
	Path     string      `json:"path"`
	Query    string      `json:"query"`
	Header   http.Header `json:"header"`
	Body     string      `json:"body"`
}

// upstream is a fake service that answers every request with a description of it.
type upstream struct {
	*httptest.Server

	hits atomic.Int64
}

func startUpstream(t *testing.T, name string) *upstream {
	t.Helper()

	u := &upstream{}

	u.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u.hits.Add(1)

		body, _ := io.ReadAll(r.Body)

		w.Header().Set("Content-Type", "application/json")

		_ = json.NewEncoder(w).Encode(seen{
			Upstream: name,
			Method:   r.Method,
			Path:     r.URL.Path,
			Query:    r.URL.RawQuery,
			Header:   r.Header,
			Body:     string(body),
		})
	}))
	t.Cleanup(u.Close)

	return u
}

// gateway is the real router (middlewares included) in front of three echo upstreams.
type gateway struct {
	http.Handler

	identity, trading, ledger *upstream
	hub                       *ws.Hub
}

func newGateway(t *testing.T, idp *mocks.Identity) *gateway {
	t.Helper()

	g := &gateway{
		identity: startUpstream(t, identityName),
		trading:  startUpstream(t, tradingName),
		ledger:   startUpstream(t, ledgerName),
	}

	g.Handler, g.hub = newRouter(t, idp, controller.Config{
		IdentityURL: g.identity.URL,
		TradingURL:  g.trading.URL,
		LedgerURL:   g.ledger.URL,
	})

	return g
}

func newRouter(t *testing.T, idp *mocks.Identity, cfg controller.Config) (http.Handler, *ws.Hub) {
	t.Helper()

	hub := ws.NewHub(ws.PingInterval)
	t.Cleanup(hub.Close)

	r := router.New()

	err := controller.GatewayRouter(r, usecase.New(idp), hub, cfg, sharedMetrics())
	require.NoError(t, err)

	return r, hub
}

func (g *gateway) totalHits() int64 {
	return g.identity.hits.Load() + g.trading.hits.Load() + g.ledger.hits.Load()
}

// identityKnowsAlice makes the session cookie "alice-session" and the API key "alice-key" valid.
func identityKnowsAlice(t *testing.T) *mocks.Identity {
	t.Helper()

	idp := mocks.NewIdentity(t)
	idp.EXPECT().Authenticate(Any, "alice-session").Return(alice, nil).Maybe()
	idp.EXPECT().AuthenticateAPIKey(Any, "alice-key").Return(alice, nil).Maybe()

	return idp
}

func withSession(token string) func(*http.Request) {
	return func(r *http.Request) { r.AddCookie(&http.Cookie{Name: domain.SessionCookie, Value: token}) }
}

func withCookie(name, value string) func(*http.Request) {
	return func(r *http.Request) { r.AddCookie(&http.Cookie{Name: name, Value: value}) }
}

func withHeader(key, value string) func(*http.Request) {
	return func(r *http.Request) { r.Header.Add(key, value) }
}

func do(t *testing.T, h http.Handler, method, target, body string, mutate ...func(*http.Request)) *httptest.ResponseRecorder {
	t.Helper()

	r := httptest.NewRequestWithContext(t.Context(), method, target, strings.NewReader(body))
	for _, m := range mutate {
		m(r)
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)

	return rec
}

func echo(t *testing.T, rec *httptest.ResponseRecorder) seen {
	t.Helper()

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var s seen
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &s), rec.Body.String())

	return s
}

func Test_Proxy_RoutesByPrefix(t *testing.T) {
	g := newGateway(t, identityKnowsAlice(t))

	tests := []struct {
		method   string
		path     string
		upstream string
	}{
		{http.MethodPost, "/api/v1/auth/register/start", identityName},
		{http.MethodPost, "/api/v1/auth/login", identityName},
		{http.MethodPost, "/api/v1/auth/logout", identityName},
		{http.MethodGet, "/api/v1/me", identityName},
		{http.MethodPut, "/api/v1/me/maker-mode", identityName},
		{http.MethodGet, "/api/v1/makers", identityName},

		{http.MethodGet, "/api/v1/desk", tradingName},
		{http.MethodPost, "/api/v1/rfqs", tradingName},
		{http.MethodGet, "/api/v1/rfqs/mine", tradingName},
		{http.MethodGet, "/api/v1/rfqs/incoming", tradingName},
		{http.MethodDelete, "/api/v1/rfqs/9f1c", tradingName},
		{http.MethodPost, "/api/v1/rfqs/9f1c/quotes", tradingName},
		{http.MethodPost, "/api/v1/quotes/7a2b/accept", tradingName},
		{http.MethodGet, "/api/v1/trades", tradingName},

		{http.MethodGet, "/api/v1/instruments", ledgerName},
		{http.MethodGet, "/api/v1/wallet/balances", ledgerName},
		{http.MethodGet, "/api/v1/wallet/incoming", ledgerName},
		{http.MethodPost, "/api/v1/wallet/incoming/00ab/accept", ledgerName},
		{http.MethodPost, "/api/v1/wallet/withdraw", ledgerName},
		{http.MethodPost, "/api/v1/wallet/faucet", ledgerName},
		{http.MethodGet, "/api/v1/tx/pending", ledgerName},
		{http.MethodPost, "/api/v1/tx/3c4d/execute", ledgerName},
	}

	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			rec := do(t, g, tt.method, tt.path+"?limit=5", `{"k":"v"}`, withSession("alice-session"))

			got := echo(t, rec)
			require.Equal(t, tt.upstream, got.Upstream)
			require.Equal(t, tt.method, got.Method)
			require.Equal(t, tt.path, got.Path, "the path reaches the service as the client sent it")
			require.Equal(t, "limit=5", got.Query)
			require.JSONEq(t, `{"k":"v"}`, got.Body)
		})
	}
}

func Test_Proxy_StripsSpoofedHeaders(t *testing.T) {
	g := newGateway(t, identityKnowsAlice(t))

	rec := do(t, g, http.MethodGet, "/api/v1/rfqs/mine", "",
		withSession("alice-session"),
		withHeader(principal.HeaderUserID, uuid.NewString()),
		withHeader(principal.HeaderPartyID, "evil"),
		withHeader(principal.HeaderPartyHint, "evil"),
		withHeader("X-User-Role", "admin"),
		withHeader("X-Other", "kept"),
	)

	got := echo(t, rec)
	require.Equal(t, tradingName, got.Upstream)
	require.Equal(t, []string{alice.UserID.String()}, got.Header.Values(principal.HeaderUserID))
	require.Equal(t, []string{alice.PartyID}, got.Header.Values(principal.HeaderPartyID), "the party is the principal's, not the client's")
	require.Equal(t, []string{alice.Hint}, got.Header.Values(principal.HeaderPartyHint))
	require.Empty(t, got.Header.Values("X-User-Role"), "any X-User-* header is the gateway's to set")
	require.Equal(t, []string{"kept"}, got.Header.Values("X-Other"), "other headers pass through")
}

func Test_Proxy_ConnectionHeaderCannotDropPrincipal(t *testing.T) {
	g := newGateway(t, identityKnowsAlice(t))

	// A proxy that adds its headers first and strips hop-by-hop headers afterwards lets the client delete them
	// by naming them in Connection. The principal must survive that.
	rec := do(t, g, http.MethodGet, "/api/v1/trades", "",
		withSession("alice-session"),
		withHeader("Connection", "X-User-Id, X-Party-Id, X-Party-Hint"),
	)

	got := echo(t, rec)
	require.Equal(t, []string{alice.UserID.String()}, got.Header.Values(principal.HeaderUserID))
	require.Equal(t, []string{alice.PartyID}, got.Header.Values(principal.HeaderPartyID))
	require.Equal(t, []string{alice.Hint}, got.Header.Values(principal.HeaderPartyHint))
}

func Test_Proxy_PublicAuthRoutes(t *testing.T) {
	// No expectations on identity: a public route must not even try to authenticate.
	g := newGateway(t, mocks.NewIdentity(t))

	rec := do(t, g, http.MethodPost, "/api/v1/auth/challenge", `{"credentialId":"c1"}`,
		withHeader(principal.HeaderUserID, uuid.NewString()),
		withHeader(principal.HeaderPartyID, "evil"),
		withHeader(principal.HeaderPartyHint, "evil"),
	)

	got := echo(t, rec)
	require.Equal(t, identityName, got.Upstream)
	require.JSONEq(t, `{"credentialId":"c1"}`, got.Body)
	require.Empty(t, got.Header.Values(principal.HeaderUserID), "nobody is vouched for on a public route")
	require.Empty(t, got.Header.Values(principal.HeaderPartyID))
	require.Empty(t, got.Header.Values(principal.HeaderPartyHint))
}

func Test_Proxy_IdentityKeepsCookieForLogout(t *testing.T) {
	g := newGateway(t, mocks.NewIdentity(t))

	rec := do(t, g, http.MethodPost, "/api/v1/auth/logout", "", withSession("alice-session"))

	got := echo(t, rec)
	require.Equal(t, identityName, got.Upstream)
	require.Equal(t, []string{domain.SessionCookie + "=alice-session"}, got.Header.Values("Cookie"), "identity reads the session cookie itself")
}

// The gateway authenticates and tells the services who is calling. A service that is never handed a session
// token or an API key cannot leak one, however exposed or compromised it is.
func Test_Proxy_TradingAndLedgerNeverSeeCredentials(t *testing.T) {
	g := newGateway(t, identityKnowsAlice(t))

	callers := map[string][]func(*http.Request){
		"browser session": {withSession("alice-session"), withCookie("theme", "dark")},
		"bot api key":     {withHeader(domain.APIKeyHeader, "alice-key")},
		"both":            {withSession("alice-session"), withHeader(domain.APIKeyHeader, "alice-key")},
	}

	targets := map[string]struct{ path, upstream string }{
		"trading": {"/api/v1/rfqs/mine", tradingName},
		"ledger":  {"/api/v1/wallet/balances", ledgerName},
	}

	for callerName, mutate := range callers {
		for targetName, target := range targets {
			t.Run(callerName+" to "+targetName, func(t *testing.T) {
				got := echo(t, do(t, g, http.MethodGet, target.path, "", mutate...))

				require.Equal(t, target.upstream, got.Upstream)
				require.Empty(t, got.Header.Values("Cookie"), "no cookie at all, not just the session")
				require.Empty(t, got.Header.Values(domain.APIKeyHeader))
				require.Equal(t, []string{alice.PartyID}, got.Header.Values(principal.HeaderPartyID), "the caller is still vouched for")
			})
		}
	}
}

// Identity authenticates API keys over gRPC; it never needs the header, not even on its public routes.
func Test_Proxy_IdentityNeverSeesAPIKey(t *testing.T) {
	g := newGateway(t, identityKnowsAlice(t))

	tests := map[string]struct{ method, path string }{
		"public route":        {http.MethodPost, "/api/v1/auth/login"},
		"authenticated route": {http.MethodGet, "/api/v1/makers"},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got := echo(t, do(t, g, tt.method, tt.path, "", withHeader(domain.APIKeyHeader, "alice-key")))

			require.Equal(t, identityName, got.Upstream)
			require.Empty(t, got.Header.Values(domain.APIKeyHeader))
		})
	}
}

func Test_Proxy_Unauthorized(t *testing.T) {
	idp := mocks.NewIdentity(t)
	idp.EXPECT().Authenticate(Any, "expired").Return(principal.Principal{}, domain.ErrUnauthorized)
	idp.EXPECT().AuthenticateAPIKey(Any, "wrong-key").Return(principal.Principal{}, domain.ErrUnauthorized)

	g := newGateway(t, idp)

	tests := map[string][]func(*http.Request){
		"no credentials":        nil,
		"expired session":       {withSession("expired")},
		"unknown api key":       {withHeader(domain.APIKeyHeader, "wrong-key")},
		"empty cookie and key":  {withSession(""), withHeader(domain.APIKeyHeader, "")},
		"spoofed headers alone": {withHeader(principal.HeaderUserID, alice.UserID.String()), withHeader(principal.HeaderPartyID, alice.PartyID), withHeader(principal.HeaderPartyHint, alice.Hint)},
	}

	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			for _, path := range []string{"/api/v1/me", "/api/v1/rfqs", "/api/v1/wallet/balances", "/api/v1/ws"} {
				rec := do(t, g, http.MethodGet, path, "", mutate...)

				require.Equal(t, http.StatusUnauthorized, rec.Code, path)
				require.Equal(t, "application/json", rec.Header().Get("Content-Type"), path)
				require.JSONEq(t, `{"error":"unauthorized"}`, rec.Body.String(), path)
			}
		})
	}

	require.Zero(t, g.totalHits(), "an unauthorized request never reaches a service")
}

func Test_Proxy_APIKeyWinsOverCookie(t *testing.T) {
	maker := principal.Principal{UserID: uuid.New(), PartyID: "makerA::1220cd", Hint: "makerA"}

	// The mock fails the test if the cookie is looked up as well.
	idp := mocks.NewIdentity(t)
	idp.EXPECT().AuthenticateAPIKey(Any, "demo-key-a").Return(maker, nil)

	g := newGateway(t, idp)

	rec := do(t, g, http.MethodGet, "/api/v1/rfqs/incoming", "",
		withSession("alice-session"),
		withHeader(domain.APIKeyHeader, "demo-key-a"),
	)

	got := echo(t, rec)
	require.Equal(t, tradingName, got.Upstream)
	require.Equal(t, []string{maker.PartyID}, got.Header.Values(principal.HeaderPartyID))
	require.Equal(t, []string{maker.Hint}, got.Header.Values(principal.HeaderPartyHint))
}

func Test_Proxy_IdentityOutage(t *testing.T) {
	idp := mocks.NewIdentity(t)
	idp.EXPECT().Authenticate(Any, "alice-session").Return(principal.Principal{}, io.ErrUnexpectedEOF)

	g := newGateway(t, idp)

	rec := do(t, g, http.MethodGet, "/api/v1/me", "", withSession("alice-session"))

	require.Equal(t, http.StatusBadGateway, rec.Code, "an outage is not a bad credential")
	require.JSONEq(t, `{"error":"upstream unavailable"}`, rec.Body.String())
	require.Zero(t, g.totalHits())
}

func Test_Proxy_UpstreamDown(t *testing.T) {
	g := newGateway(t, identityKnowsAlice(t))

	g.trading.Close() // the connection is refused from now on

	rec := do(t, g, http.MethodGet, "/api/v1/trades", "", withSession("alice-session"))

	require.Equal(t, http.StatusBadGateway, rec.Code)
	require.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	require.JSONEq(t, `{"error":"upstream unavailable"}`, rec.Body.String())

	// The other services are unaffected.
	require.Equal(t, ledgerName, echo(t, do(t, g, http.MethodGet, "/api/v1/instruments", "", withSession("alice-session"))).Upstream)
}

func Test_Proxy_PublicRouteUpstreamDown(t *testing.T) {
	g := newGateway(t, mocks.NewIdentity(t))

	g.identity.Close()

	rec := do(t, g, http.MethodPost, "/api/v1/auth/login", `{}`)

	require.Equal(t, http.StatusBadGateway, rec.Code)
	require.JSONEq(t, `{"error":"upstream unavailable"}`, rec.Body.String())
}

func Test_Proxy_PassesResponsesThrough(t *testing.T) {
	trading := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Upstream", "trading")
		w.Header().Add("Set-Cookie", "otc_session=abc; Path=/; HttpOnly; SameSite=Lax")
		w.Header().Add("Set-Cookie", "other=1; Path=/")
		w.WriteHeader(http.StatusConflict)
		_, _ = io.WriteString(w, `{"error":"quote is not open","code":"QUOTE_NOT_OPEN"}`)
	}))
	t.Cleanup(trading.Close)

	h, _ := newRouter(t, identityKnowsAlice(t), controller.Config{
		IdentityURL: "http://127.0.0.1:1", TradingURL: trading.URL, LedgerURL: "http://127.0.0.1:1",
	})

	rec := do(t, h, http.MethodPost, "/api/v1/quotes/7a2b/accept", "", withSession("alice-session"))

	require.Equal(t, http.StatusConflict, rec.Code, "error statuses are the service's, not the gateway's")
	require.JSONEq(t, `{"error":"quote is not open","code":"QUOTE_NOT_OPEN"}`, rec.Body.String())
	require.Equal(t, "trading", rec.Header().Get("X-Upstream"))
	require.Equal(t,
		[]string{"otc_session=abc; Path=/; HttpOnly; SameSite=Lax", "other=1; Path=/"},
		rec.Header().Values("Set-Cookie"),
		"every Set-Cookie survives",
	)
}

func Test_Proxy_UnknownAPIPaths(t *testing.T) {
	g := newGateway(t, identityKnowsAlice(t))

	paths := []string{
		"/api", "/api/", "/api/v1", "/api/v1/", "/api/v1/unknown",
		"/api/v1/rfqsX", "/api/v1/mexico", "/api/v1/makers2", "/api/v1/auth", "/api/v1/wallets",
		"/api/v2/me", "/api/v1/ws/extra",
	}

	for _, path := range paths {
		for name, mutate := range map[string][]func(*http.Request){
			"anonymous":     nil,
			"authenticated": {withSession("alice-session")},
		} {
			t.Run(name+" "+path, func(t *testing.T) {
				rec := do(t, g, http.MethodGet, path, "", mutate...)

				require.Equal(t, http.StatusNotFound, rec.Code)
				require.Equal(t, "application/json", rec.Header().Get("Content-Type"))
				require.JSONEq(t, `{"error":"not found"}`, rec.Body.String())
			})
		}
	}

	require.Zero(t, g.totalHits(), "nothing is forwarded for a path the gateway does not route")
}

func Test_Router_ServesProbesAndMetrics(t *testing.T) {
	g := newGateway(t, mocks.NewIdentity(t))

	for _, path := range []string{"/live", "/ready", "/metrics"} {
		rec := do(t, g, http.MethodGet, path, "")
		require.Equal(t, http.StatusOK, rec.Code, path)
	}
}

func Test_Router_RejectsInvalidUpstreamURL(t *testing.T) {
	valid := controller.Config{IdentityURL: "http://identity:8081", TradingURL: "http://trading:8082", LedgerURL: "http://ledger:8083"}

	tests := map[string]func(c *controller.Config){
		"identity without scheme": func(c *controller.Config) { c.IdentityURL = "identity:8081" },
		"trading empty":           func(c *controller.Config) { c.TradingURL = "" },
		"ledger not http":         func(c *controller.Config) { c.LedgerURL = "ftp://ledger:8083" },
		"ledger without host":     func(c *controller.Config) { c.LedgerURL = "http://" },
	}

	for name, damage := range tests {
		t.Run(name, func(t *testing.T) {
			cfg := valid
			damage(&cfg)

			err := controller.GatewayRouter(router.New(), usecase.New(mocks.NewIdentity(t)), ws.NewHub(ws.PingInterval), cfg, sharedMetrics())
			require.Error(t, err)
		})
	}
}
