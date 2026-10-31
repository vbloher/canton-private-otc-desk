package otel_test

import (
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
