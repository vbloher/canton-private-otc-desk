package logger_test

import (
	"testing"

	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/log/logtest"

	otelapi "go.opentelemetry.io/otel"

	"github.com/vbloher/canton-private-otc-desk/pkg/logger"
)

func TestInit_ExportsToTheGlobalLoggerProvider(t *testing.T) {
	prev := log.Logger

	t.Cleanup(func() { log.Logger = prev })

	rec := logtest.NewRecorder()
	otelapi.SetLoggerProvider(rec)

	logger.Init(logger.Config{Level: "info"})
	log.Info().Str("k", "v").Msg("hello")

	var bodies []string

	for _, r := range records(rec) {
		bodies = append(bodies, r.Body.AsString())
	}

	require.Contains(t, bodies, "hello")
}
