package otel

import (
	"context"
	"fmt"
	"runtime/debug"
	"time"

	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"

	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
	tracer_noop "go.opentelemetry.io/otel/trace/noop"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
)

type Config struct {
	ServiceName string  `env:"OTEL_SERVICE_NAME"`
	Endpoint    string  `env:"OTEL_ENDPOINT"`
	Namespace   string  `env:"OTEL_NAMESPACE"`
	InstanceID  string  `env:"OTEL_INSTANCE_ID"`
	Ratio       float64 `env:"OTEL_RATIO"        envDefault:"1.0"`
}

var shutdownTracing func(ctx context.Context) error //nolint:gochecknoglobals

func SilentModeInit() {
	otel.SetTracerProvider(tracer_noop.NewTracerProvider())
	tracer.Init(otel.Tracer(""))

	log.Info().Msg("otel: Tracer is disabled")
}

func Init(ctx context.Context, c Config) error {
	if c.Endpoint == "" {
		SilentModeInit()

		return nil
	}

	prop := propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	)

	otel.SetTextMapPropagator(prop)

	traceExporter, err := otlptracegrpc.New(ctx, otlptracegrpc.WithEndpoint(c.Endpoint), otlptracegrpc.WithInsecure())
	if err != nil {
		return fmt.Errorf("failed to create OTLP trace exporter: %w", err)
	}

	traceProvider := trace.NewTracerProvider(
		trace.WithBatcher(traceExporter, trace.WithBatchTimeout(time.Second)),
		trace.WithSampler(trace.ParentBased(trace.TraceIDRatioBased(c.Ratio))),
		trace.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String(c.ServiceName),
			semconv.ServiceNamespaceKey.String(c.Namespace),
			semconv.ServiceInstanceIDKey.String(c.InstanceID),
			semconv.ServiceVersionKey.String(version()),
		)),
	)

	shutdownTracing = traceProvider.Shutdown

	otel.SetTracerProvider(traceProvider)
	tracer.Init(otel.Tracer(""))

	return nil
}

func version() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}

	return info.Main.Version
}

func Close() {
	if shutdownTracing == nil {
		return
	}

	err := shutdownTracing(context.Background())
	if err != nil {
		log.Error().Err(err).Msg("otel: failed to shutdown tracing")
	}

	log.Info().Msg("otel: closed")
}
