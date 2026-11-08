package otel

import (
	"fmt"
	"runtime"

	"github.com/grafana/pyroscope-go"
	"go.opentelemetry.io/otel/sdk/trace"

	otelpyroscope "github.com/grafana/otel-profiling-go"
	oteltrace "go.opentelemetry.io/otel/trace"
)

const mutexProfileFraction = 5

//nolint:ireturn
func withProfiler(c Config, provider *trace.TracerProvider) (oteltrace.TracerProvider, func() error, error) {
	if c.ProfilerEndpoint == "" {
		return provider, func() error { return nil }, nil
	}

	profiler, err := startProfiler(c)
	if err != nil {
		return nil, nil, err
	}

	return otelpyroscope.NewTracerProvider(provider), profiler.Stop, nil
}

func startProfiler(c Config) (*pyroscope.Profiler, error) {
	runtime.SetMutexProfileFraction(mutexProfileFraction)

	tags := map[string]string{}
	if c.ProfilerEnv != "" {
		tags["env"] = c.ProfilerEnv
	}

	profiler, err := pyroscope.Start(pyroscope.Config{
		ApplicationName: c.ServiceName,
		ServerAddress:   c.ProfilerEndpoint,
		Tags:            tags,
		DisableGCRuns:   true,
		ProfileTypes: []pyroscope.ProfileType{
			pyroscope.ProfileCPU,
			pyroscope.ProfileAllocObjects,
			pyroscope.ProfileAllocSpace,
			pyroscope.ProfileInuseObjects,
			pyroscope.ProfileInuseSpace,
			pyroscope.ProfileGoroutines,
			pyroscope.ProfileMutexCount,
			pyroscope.ProfileMutexDuration,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("pyroscope.Start: %w", err)
	}

	return profiler, nil
}
