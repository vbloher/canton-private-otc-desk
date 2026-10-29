package tracer

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

var tracer trace.Tracer = noop.NewTracerProvider().Tracer("") //nolint:gochecknoglobals

type suppressKey struct{}

func Init(t trace.Tracer) {
	tracer = t
}

//nolint:ireturn
func Start(ctx context.Context, spanName string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	if suppressed, _ := ctx.Value(suppressKey{}).(bool); suppressed {
		return ctx, noop.Span{}
	}

	return tracer.Start(ctx, spanName, opts...) //nolint:spancheck
}

func Suppress(ctx context.Context) context.Context {
	return context.WithValue(ctx, suppressKey{}, true)
}

func Inject(ctx context.Context, carrier map[string]string) {
	otel.GetTextMapPropagator().Inject(ctx, propagation.MapCarrier(carrier))
}

func Extract(ctx context.Context, carrier map[string]string) context.Context {
	ctx = otel.GetTextMapPropagator().Extract(ctx, propagation.MapCarrier(carrier))

	if trace.SpanContextFromContext(ctx).IsRemote() {
		return context.WithValue(ctx, suppressKey{}, false)
	}

	return ctx
}
