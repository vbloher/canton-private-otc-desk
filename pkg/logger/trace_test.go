package logger_test

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"

	"github.com/vbloher/canton-private-otc-desk/pkg/logger"
)

const (
	traceID = "0102030405060708090a0b0c0d0e0f10"
	spanID  = "0102030405060708"
)

func spanContext(t *testing.T, flags trace.TraceFlags) context.Context {
	t.Helper()

	tid, err := trace.TraceIDFromHex(traceID)
	require.NoError(t, err)

	sid, err := trace.SpanIDFromHex(spanID)
	require.NoError(t, err)

	sc := trace.NewSpanContext(trace.SpanContextConfig{TraceID: tid, SpanID: sid, TraceFlags: flags})

	return trace.ContextWithSpanContext(t.Context(), sc)
}

func decode(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()

	var fields map[string]any

	require.NoError(t, json.Unmarshal(buf.Bytes(), &fields))

	return fields
}

func TestTraceHook_AddsSampledSpanIDs(t *testing.T) {
	var buf bytes.Buffer

	l := zerolog.New(&buf).Hook(logger.TraceHook{})
	l.Info().Ctx(spanContext(t, trace.FlagsSampled)).Msg("x")

	fields := decode(t, &buf)
	require.Equal(t, traceID, fields["trace_id"])
	require.Equal(t, spanID, fields["span_id"])
}

func TestTraceHook_SkipsUnsampledSpan(t *testing.T) {
	var buf bytes.Buffer

	l := zerolog.New(&buf).Hook(logger.TraceHook{})
	l.Info().Ctx(spanContext(t, 0)).Msg("x")

	fields := decode(t, &buf)
	require.NotContains(t, fields, "trace_id")
	require.NotContains(t, fields, "span_id")
}

func TestTraceHook_SkipsEventWithoutContext(t *testing.T) {
	var buf bytes.Buffer

	l := zerolog.New(&buf).Hook(logger.TraceHook{})
	l.Info().Msg("x")

	fields := decode(t, &buf)
	require.NotContains(t, fields, "trace_id")
	require.NotContains(t, fields, "span_id")
}
