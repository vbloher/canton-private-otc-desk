package http

import (
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/vbloher/canton-private-otc-desk/pkg/logger"
	"github.com/vbloher/canton-private-otc-desk/pkg/metrics"
	"github.com/vbloher/canton-private-otc-desk/pkg/otel"
	"github.com/vbloher/canton-private-otc-desk/pkg/render"
	"github.com/vbloher/canton-private-otc-desk/services/gateway/internal/controller/ws"
	"github.com/vbloher/canton-private-otc-desk/services/gateway/internal/usecase"
)

type Config struct {
	IdentityURL string `env:"IDENTITY_URL" envDefault:"http://localhost:8081"`
	TradingURL  string `env:"TRADING_URL"  envDefault:"http://localhost:8082"`
	LedgerURL   string `env:"LEDGER_URL"   envDefault:"http://localhost:8083"`
}

func GatewayRouter(r *chi.Mux, uc *usecase.UseCase, hub *ws.Hub, c Config, m *metrics.HTTPServer) error {
	transport := newTransport()

	identity, err := newProxy(c.IdentityURL, transport, keepCookie)
	if err != nil {
		return fmt.Errorf("identity newProxy: %w", err)
	}

	trading, err := newProxy(c.TradingURL, transport, dropCookie)
	if err != nil {
		return fmt.Errorf("trading newProxy: %w", err)
	}

	ledger, err := newProxy(c.LedgerURL, transport, dropCookie)
	if err != nil {
		return fmt.Errorf("ledger newProxy: %w", err)
	}

	auth := authenticate(uc)

	r.Handle("/metrics", promhttp.Handler())

	r.With(otel.Middleware, auth).Method(http.MethodGet, "/api/v1/ws", ws.NewHandler(hub))

	r.Group(func(r chi.Router) {
		r.Use(otel.Middleware)
		r.Use(logger.Middleware)
		r.Use(metrics.NewMiddleware(m))

		r.Handle("/api/v1/auth/*", identity)

		r.Group(func(r chi.Router) {
			r.Use(auth)

			for _, prefix := range []string{"/api/v1/me", "/api/v1/makers"} {
				mount(r, prefix, identity)
			}

			for _, prefix := range []string{"/api/v1/desk", "/api/v1/rfqs", "/api/v1/quotes", "/api/v1/trades"} {
				mount(r, prefix, trading)
			}

			for _, prefix := range []string{"/api/v1/instruments", "/api/v1/wallet", "/api/v1/tx"} {
				mount(r, prefix, ledger)
			}
		})

		r.HandleFunc("/api", notFound)
		r.HandleFunc("/api/*", notFound)
	})

	return nil
}

func mount(r chi.Router, prefix string, h http.Handler) {
	r.Handle(prefix, h)
	r.Handle(prefix+"/*", h)
}

func notFound(w http.ResponseWriter, _ *http.Request) {
	render.JSON(w, render.Err{Error: "not found"}, http.StatusNotFound)
}
