package http

import (
	"fmt"
	"net/http"
	"net/http/httputil"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/vbloher/canton-private-otc-desk/pkg/logger"
	"github.com/vbloher/canton-private-otc-desk/pkg/metrics"
	"github.com/vbloher/canton-private-otc-desk/pkg/otel"
	"github.com/vbloher/canton-private-otc-desk/pkg/render"
	"github.com/vbloher/canton-private-otc-desk/services/gateway/internal/controller/ws"
	"github.com/vbloher/canton-private-otc-desk/services/gateway/internal/usecase"
)

// Config is what the router takes from the environment: where the services are, and which other browser
// origins may call the API.
type Config struct {
	IdentityURL string `env:"IDENTITY_URL" envDefault:"http://localhost:8081"`
	TradingURL  string `env:"TRADING_URL"  envDefault:"http://localhost:8082"`
	LedgerURL   string `env:"LEDGER_URL"   envDefault:"http://localhost:8083"`

	// CORSOrigins are the origins, besides the gateway's own, whose pages may call the API from a browser: exact
	// scheme://host[:port], no path, e.g. https://<owner>.github.io. Empty means same-origin only.
	CORSOrigins []string `env:"CORS_ORIGINS"`
}

// GatewayRouter authenticates every request under /api/v1 except /api/v1/auth/, then hands it to the service
// that owns the path, or to the WebSocket hub. Whatever else is under /api answers 404.
// The services get the caller as X-User-Id / X-Party-Id / X-Party-Hint and none of its secrets: no API key
// anywhere, and the session (cookie or bearer token) only for identity.
//
// It registers the routes on r and returns the handler to serve: r behind the layers that must come first, CORS
// (preflights are answered before authentication and routing) and the no-store policy for everything under /api.
func GatewayRouter(
	r *chi.Mux, uc *usecase.UseCase, hub *ws.Hub, c Config, m *metrics.HTTPServer,
) (http.Handler, error) {
	policy, err := newCORS(c.CORSOrigins)
	if err != nil {
		return nil, fmt.Errorf("newCORS: %w", err)
	}

	up, err := newUpstreams(c)
	if err != nil {
		return nil, fmt.Errorf("newUpstreams: %w", err)
	}

	auth := authenticate(uc, requestCredentials)

	r.Handle("/metrics", promhttp.Handler())

	// The socket route stays beside the logging and metrics group: a socket lives for hours, while those
	// middlewares record one log line and one duration sample (the buckets end at 120 s) when a request ends.
	// The hub logs the sockets it drops. Authentication and OpenTelemetry apply as everywhere; a browser can only
	// offer its session token as a subprotocol, which is why the socket has credentials of its own.
	r.With(otel.Middleware, authenticate(uc, socketCredentials)).
		Method(http.MethodGet, "/api/v1/ws", ws.NewHandler(hub, policy.originPatterns()...))

	r.Group(func(r chi.Router) {
		r.Use(otel.Middleware)
		r.Use(logger.Middleware)
		r.Use(metrics.NewMiddleware(m))

		// Public: the caller is not signed in yet. Identity checks the credentials on these itself.
		r.Handle("/api/v1/auth/*", up.identity)

		r.Group(func(r chi.Router) {
			r.Use(auth)

			for _, prefix := range []string{"/api/v1/me", "/api/v1/makers"} {
				mount(r, prefix, up.identity)
			}

			for _, prefix := range []string{"/api/v1/desk", "/api/v1/rfqs", "/api/v1/quotes", "/api/v1/trades"} {
				mount(r, prefix, up.trading)
			}

			for _, prefix := range []string{"/api/v1/instruments", "/api/v1/wallet", "/api/v1/tx"} {
				mount(r, prefix, up.ledger)
			}
		})

		r.HandleFunc("/api", notFound)
		r.HandleFunc("/api/*", notFound)
	})

	return noStore(policy.middleware(r)), nil
}

// upstreams are the reverse proxies to the three services.
type upstreams struct {
	identity, trading, ledger *httputil.ReverseProxy
}

func newUpstreams(c Config) (upstreams, error) {
	transport := newTransport()

	identity, err := newProxy(c.IdentityURL, transport, keepSession)
	if err != nil {
		return upstreams{}, fmt.Errorf("identity newProxy: %w", err)
	}

	trading, err := newProxy(c.TradingURL, transport, dropSession)
	if err != nil {
		return upstreams{}, fmt.Errorf("trading newProxy: %w", err)
	}

	ledger, err := newProxy(c.LedgerURL, transport, dropSession)
	if err != nil {
		return upstreams{}, fmt.Errorf("ledger newProxy: %w", err)
	}

	return upstreams{identity: identity, trading: trading, ledger: ledger}, nil
}

// mount routes the path itself and everything below it, but not look-alikes such as /api/v1/mexico for /api/v1/me.
func mount(r chi.Router, prefix string, h http.Handler) {
	r.Handle(prefix, h)
	r.Handle(prefix+"/*", h)
}

func notFound(w http.ResponseWriter, _ *http.Request) {
	render.JSON(w, render.Err{Error: "not found"}, http.StatusNotFound)
}
