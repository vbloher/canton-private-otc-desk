package logger_test

import (
	"context"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/log/logtest"
	"go.opentelemetry.io/otel/trace"

	otellog "go.opentelemetry.io/otel/log"

	"github.com/vbloher/canton-private-otc-desk/pkg/logger"
)

func newRecorderWriter(opts ...logtest.Option) (*logtest.Recorder, zerolog.LevelWriter) {
	rec := logtest.NewRecorder(opts...)

	return rec, logger.NewOTelWriter(rec.Logger("test"))
}

func records(rec *logtest.Recorder) []logtest.Record {
	var out []logtest.Record

	for _, rs := range rec.Result() {
		out = append(out, rs...)
	}

	return out
}

func attributes(r logtest.Record) map[string]string {
	attrs := map[string]string{}

	for _, kv := range r.Attributes {
		attrs[string(kv.Key)] = kv.Value.String()
	}

	return attrs
}

func TestOTelWriter_MapsLine(t *testing.T) {
	rec, w := newRecorderWriter()

	l := zerolog.New(w).With().Timestamp().Logger().Hook(logger.TraceHook{})
	l.Error().Ctx(spanContext(t, trace.FlagsSampled)).Int("code", 502).Str("method", "POST /api/v1/x").Msg("boom")

	rs := records(rec)
	require.Len(t, rs, 1)
	require.Equal(t, "boom", rs[0].Body.AsString())
	require.Equal(t, otellog.SeverityError, rs[0].Severity)
	require.Equal(t, "error", rs[0].SeverityText)
	require.Equal(t, map[string]string{"code": "502", "method": "POST /api/v1/x"}, attributes(rs[0]))

	sc := trace.SpanContextFromContext(rs[0].Context)
	require.Equal(t, traceID, sc.TraceID().String())
	require.Equal(t, spanID, sc.SpanID().String())
}

func TestOTelWriter_MapsLevels(t *testing.T) {
	levels := map[zerolog.Level]otellog.Severity{
		zerolog.TraceLevel: otellog.SeverityTrace,
		zerolog.DebugLevel: otellog.SeverityDebug,
		zerolog.InfoLevel:  otellog.SeverityInfo,
		zerolog.WarnLevel:  otellog.SeverityWarn,
		zerolog.ErrorLevel: otellog.SeverityError,
		zerolog.FatalLevel: otellog.SeverityFatal,
	}

	for level, severity := range levels {
		rec, w := newRecorderWriter()

		_, err := w.WriteLevel(level, []byte(`{"level":"`+level.String()+`","message":"x"}`))
		require.NoError(t, err)

		rs := records(rec)
		require.Len(t, rs, 1)
		require.Equal(t, severity, rs[0].Severity, level.String())
	}
}

func TestOTelWriter_StringifiesNestedValues(t *testing.T) {
	rec, w := newRecorderWriter()

	l := zerolog.New(w)
	l.Info().Dict("d", zerolog.Dict().Str("a", "b")).Msg("x")

	rs := records(rec)
	require.Len(t, rs, 1)
	require.JSONEq(t, `{"a":"b"}`, attributes(rs[0])["d"])
}

func TestOTelWriter_IgnoresInvalidJSON(t *testing.T) {
	rec, w := newRecorderWriter()

	n, err := w.WriteLevel(zerolog.InfoLevel, []byte("not json"))
	require.NoError(t, err)
	require.Equal(t, 8, n)
	require.Empty(t, records(rec))
}

func TestOTelWriter_DisabledLoggerSkipsRecord(t *testing.T) {
	rec, w := newRecorderWriter(logtest.WithEnabledFunc(func(context.Context, otellog.EnabledParameters) bool {
		return false
	}))

	line := []byte(`{"level":"info","message":"x"}`)

	n, err := w.WriteLevel(zerolog.InfoLevel, line)
	require.NoError(t, err)
	require.Equal(t, len(line), n)
	require.Empty(t, records(rec))
}

func TestOTelWriter_UsesFieldsAsBodyWithoutMessage(t *testing.T) {
	rec, w := newRecorderWriter()

	l := zerolog.New(w).With().Timestamp().Logger()
	l.Info().Int("code", 401).Str("method", "GET /api/v1/instruments").Send()

	rs := records(rec)
	require.Len(t, rs, 1)
	require.JSONEq(t, `{"code":401,"method":"GET /api/v1/instruments"}`, rs[0].Body.AsString())
	require.Equal(t, map[string]string{"code": "401", "method": "GET /api/v1/instruments"}, attributes(rs[0]))
}
