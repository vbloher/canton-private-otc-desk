package tracer_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
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

func TestStartRoot_RecordsInSuppressedContext(t *testing.T) {
	recorder := setup(t)

	ctx, root := tracer.StartRoot(tracer.Suppress(t.Context()), "usecase ExpireRFQ")
	_, child := tracer.Start(ctx, "adapter postgres UpdateRFQ")
	child.End()
	root.End()

	spans := recorder.Ended()
	require.Len(t, spans, 2)
	require.False(t, spans[1].Parent().IsValid())
	require.Equal(t, spans[1].SpanContext().SpanID(), spans[0].Parent().SpanID())
}

func TestStartRoot_IgnoresParent(t *testing.T) {
	recorder := setup(t)

	ctx, parent := tracer.Start(t.Context(), "request")
	_, root := tracer.StartRoot(ctx, "usecase ExpireRFQ")
	root.End()
	parent.End()

	spans := recorder.Ended()
	require.Len(t, spans, 2)
	require.NotEqual(t, spans[0].SpanContext().TraceID(), spans[1].SpanContext().TraceID())
}

func TestEnd_RecordsError(t *testing.T) {
	recorder := setup(t)

	_, span := tracer.StartRoot(t.Context(), "usecase ExpireRFQ")
	tracer.End(span, errors.New("boom"))

	spans := recorder.Ended()
	require.Len(t, spans, 1)
	require.Equal(t, codes.Error, spans[0].Status().Code)
	require.Equal(t, "boom", spans[0].Status().Description)
	require.Len(t, spans[0].Events(), 1)
}

func TestEnd_LeavesStatusUnsetOnSuccess(t *testing.T) {
	recorder := setup(t)

	_, span := tracer.StartRoot(t.Context(), "usecase ExpireRFQ")
	tracer.End(span, nil)

	spans := recorder.Ended()
	require.Len(t, spans, 1)
	require.Equal(t, codes.Unset, spans[0].Status().Code)
}
