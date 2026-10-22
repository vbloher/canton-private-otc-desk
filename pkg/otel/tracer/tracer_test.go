package tracer_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
)

func setup(t *testing.T) *tracetest.SpanRecorder {
	t.Helper()

	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))

	otel.SetTextMapPropagator(propagation.TraceContext{})
	tracer.Init(provider.Tracer(""))

	return recorder
}

func TestStart_Suppressed(t *testing.T) {
	recorder := setup(t)

	_, span := tracer.Start(tracer.Suppress(t.Context()), "poll")
	span.End()

	require.Empty(t, recorder.Ended())
}

func TestExtract_ContinuesTraceInSuppressedContext(t *testing.T) {
	recorder := setup(t)

	requestCtx, request := tracer.Start(t.Context(), "request")

	headers := map[string]string{}
	tracer.Inject(requestCtx, headers)
	request.End()

	_, produce := tracer.Start(tracer.Extract(tracer.Suppress(t.Context()), headers), "produce")
	produce.End()

	spans := recorder.Ended()
	require.Len(t, spans, 2)
	require.Equal(t, spans[0].SpanContext().TraceID(), spans[1].SpanContext().TraceID())
	require.Equal(t, spans[0].SpanContext().SpanID(), spans[1].Parent().SpanID())
}

func TestExtract_EmptyHeadersStaySuppressed(t *testing.T) {
	recorder := setup(t)

	_, span := tracer.Start(tracer.Extract(tracer.Suppress(context.Background()), map[string]string{}), "produce")
	span.End()

	require.Empty(t, recorder.Ended())
}
