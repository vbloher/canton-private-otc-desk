package otel

import (
	"net/http"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/trace"

	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"

	"github.com/vbloher/canton-private-otc-desk/pkg/router"
)

func Middleware(next http.Handler) http.Handler {
	return otelhttp.NewHandler(withRoute(next), "http server",
		otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
			return r.Method + " " + router.ExtractPath(r.Context())
		}),
	)
}

func withRoute(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)

		trace.SpanFromContext(r.Context()).SetAttributes(semconv.HTTPRoute(router.ExtractPath(r.Context())))
	})
}
