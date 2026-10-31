package logger_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	otelapi "go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/vbloher/canton-private-otc-desk/pkg/logger"
	"github.com/vbloher/canton-private-otc-desk/pkg/otel"
)

func TestMiddleware_LogsTraceID(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	otelapi.SetTracerProvider(sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder)))

	var buf bytes.Buffer

	prev := log.Logger
	log.Logger = zerolog.New(&buf).Hook(logger.TraceHook{})

	t.Cleanup(func() { log.Logger = prev })

	r := chi.NewRouter()
	r.Use(otel.Middleware)
	r.Use(logger.Middleware)
	r.Get("/x", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })

	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/x", nil))

	spans := recorder.Ended()
	require.Len(t, spans, 1)

	fields := decode(t, &buf)
	require.Equal(t, spans[0].SpanContext().TraceID().String(), fields["trace_id"])
	require.Equal(t, "GET /x", fields["method"])
}
