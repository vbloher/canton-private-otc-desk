package http

import (
	"fmt"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	nethttpmiddleware "github.com/oapi-codegen/nethttp-middleware"

	http_server "github.com/vbloher/canton-private-otc-desk/gen/http/wallet_v1/server"
	"github.com/vbloher/canton-private-otc-desk/pkg/logger"
	"github.com/vbloher/canton-private-otc-desk/pkg/metrics"
	"github.com/vbloher/canton-private-otc-desk/pkg/otel"
	"github.com/vbloher/canton-private-otc-desk/pkg/principal"
	ver1 "github.com/vbloher/canton-private-otc-desk/services/ledger/internal/controller/http/v1"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/usecase"
)

// WalletRouter serves the wallet API: every route reads its caller from the headers the
// gateway forwards.
func WalletRouter(r *chi.Mux, uc *usecase.UseCase, m *metrics.HTTPServer) error {
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

	handler := http_server.NewStrictHandlerWithOptions(ver1.New(uc), nil, http_server.StrictHTTPServerOptions{
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
