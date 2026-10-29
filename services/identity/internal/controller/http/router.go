package http

import (
	"fmt"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	nethttpmiddleware "github.com/oapi-codegen/nethttp-middleware"

	http_server "github.com/vbloher/canton-private-otc-desk/gen/http/identity_v1/server"
	"github.com/vbloher/canton-private-otc-desk/pkg/logger"
	"github.com/vbloher/canton-private-otc-desk/pkg/metrics"
	"github.com/vbloher/canton-private-otc-desk/pkg/otel"
	"github.com/vbloher/canton-private-otc-desk/pkg/principal"
	ver1 "github.com/vbloher/canton-private-otc-desk/services/identity/internal/controller/http/v1"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/usecase"
)

type Config struct {
	CookieSecure bool `env:"COOKIE_SECURE" envDefault:"false"`
}

func IdentityRouter(r *chi.Mux, uc *usecase.UseCase, m *metrics.HTTPServer, c Config) error {
	spec, err := http_server.GetSpec()
	if err != nil {
		return fmt.Errorf("http_server.GetSpec: %w", err)
	}

	baseURL, err := spec.Servers.BasePath()
	if err != nil {
		return fmt.Errorf("spec.Servers.BasePath: %w", err)
	}

	validatorOptions := &nethttpmiddleware.Options{
		ErrorHandlerWithOpts: ver1.ValidationErrorHandler,
		DoNotValidateServers: true,
		Prefix:               baseURL,
	}
	validatorOptions.Options.WithCustomSchemaErrorFunc(ver1.SchemaErrorMessage)

	handlers := ver1.New(uc, c.CookieSecure)

	handler := http_server.NewStrictHandlerWithOptions(handlers, nil, http_server.StrictHTTPServerOptions{
		RequestErrorHandlerFunc:  ver1.RequestErrorHandler,
		ResponseErrorHandlerFunc: ver1.ResponseErrorHandler,
	})

	r.Handle("/metrics", promhttp.Handler())

	r.Group(func(r chi.Router) {
		r.Use(otel.Middleware)
		r.Use(logger.Middleware)
		r.Use(metrics.NewMiddleware(m))
		r.Use(principal.Middleware)
		r.Use(nethttpmiddleware.OapiRequestValidatorWithOptions(spec, validatorOptions))

		http_server.HandlerWithOptions(handler, http_server.ChiServerOptions{
			BaseURL:          baseURL,
			BaseRouter:       r,
			ErrorHandlerFunc: ver1.RequestErrorHandler,
		})
	})

	return nil
}
