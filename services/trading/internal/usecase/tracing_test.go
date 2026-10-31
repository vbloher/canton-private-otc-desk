package usecase_test

import (
	"testing"

	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
)

func recordSpans(t *testing.T) *tracetest.SpanRecorder {
	t.Helper()

	recorder := tracetest.NewSpanRecorder()
	tracer.Init(sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder)).Tracer(""))

	return recorder
}

func roots(spans []sdktrace.ReadOnlySpan) []sdktrace.ReadOnlySpan {
	var out []sdktrace.ReadOnlySpan

	for _, s := range spans {
		if !s.Parent().IsValid() {
			out = append(out, s)
		}
	}

	return out
}
