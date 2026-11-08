package otel_test

import (
	"runtime"
	"runtime/pprof"
	"testing"

	"github.com/stretchr/testify/require"

	otelapi "go.opentelemetry.io/otel"
	sdklog "go.opentelemetry.io/otel/sdk/log"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel"
)

func TestInit_InstallsLoggerProvider(t *testing.T) {
	require.NoError(t, otel.Init(t.Context(), otel.Config{ServiceName: "test", Endpoint: "127.0.0.1:4317"}))

	t.Cleanup(otel.Close)

	_, ok := otelapi.GetLoggerProvider().(*sdklog.LoggerProvider)
	require.True(t, ok)
}

func TestInit_WithProfilerEndpointSamplesMutexesAndLabelsSpans(t *testing.T) {
	t.Cleanup(func() { runtime.SetMutexProfileFraction(0) })

	require.NoError(t, otel.Init(t.Context(), otel.Config{
		ServiceName:      "test",
		Endpoint:         "127.0.0.1:4317",
		ProfilerEndpoint: "http://127.0.0.1:4040",
		ProfilerEnv:      "test",
	}))

	t.Cleanup(otel.Close)

	require.Equal(t, 5, runtime.SetMutexProfileFraction(-1))

	ctx, span := otelapi.Tracer("").Start(t.Context(), "request")
	defer span.End()

	name, ok := pprof.Label(ctx, "span_name")
	require.True(t, ok)
	require.Equal(t, "request", name)
}

func TestInit_WithoutProfilerEndpointLeavesProfilingOff(t *testing.T) {
	require.NoError(t, otel.Init(t.Context(), otel.Config{ServiceName: "test", Endpoint: "127.0.0.1:4317"}))

	t.Cleanup(otel.Close)

	require.Equal(t, 0, runtime.SetMutexProfileFraction(-1))

	ctx, span := otelapi.Tracer("").Start(t.Context(), "request")
	defer span.End()

	_, ok := pprof.Label(ctx, "span_name")
	require.False(t, ok)
}
