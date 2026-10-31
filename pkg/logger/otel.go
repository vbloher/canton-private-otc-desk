package logger

import (
	"context"
	"encoding/json"

	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	otellog "go.opentelemetry.io/otel/log"
)

type otelWriter struct {
	logger otellog.Logger
}

func NewOTelWriter(l otellog.Logger) zerolog.LevelWriter {
	return otelWriter{logger: l}
}

func (w otelWriter) Write(p []byte) (int, error) {
	return w.WriteLevel(zerolog.NoLevel, p)
}

func (w otelWriter) WriteLevel(level zerolog.Level, p []byte) (int, error) {
	sev := severity(level)

	if !w.logger.Enabled(context.Background(), otellog.EnabledParameters{Severity: sev}) {
		return len(p), nil
	}

	var fields map[string]any

	if json.Unmarshal(p, &fields) == nil {
		w.emit(sev, level.String(), fields)
	}

	return len(p), nil
}

func (w otelWriter) emit(sev otellog.Severity, severityText string, fields map[string]any) {
	var record otellog.Record

	record.SetSeverity(sev)
	record.SetSeverityText(severityText)

	if msg, ok := fields[zerolog.MessageFieldName].(string); ok {
		record.SetBody(attribute.StringValue(msg))
	}

	ctx := withSpan(context.Background(), fields)

	for _, key := range []string{
		zerolog.LevelFieldName, zerolog.MessageFieldName, zerolog.TimestampFieldName, traceIDField, spanIDField,
	} {
		delete(fields, key)
	}

	for key, value := range fields {
		record.AddAttributes(attribute.String(key, stringify(value)))
	}

	w.logger.Emit(ctx, record)
}

func severity(level zerolog.Level) otellog.Severity {
	switch level {
	case zerolog.TraceLevel:
		return otellog.SeverityTrace
	case zerolog.DebugLevel:
		return otellog.SeverityDebug
	case zerolog.InfoLevel:
		return otellog.SeverityInfo
	case zerolog.WarnLevel:
		return otellog.SeverityWarn
	case zerolog.ErrorLevel:
		return otellog.SeverityError
	case zerolog.FatalLevel, zerolog.PanicLevel:
		return otellog.SeverityFatal
	case zerolog.NoLevel, zerolog.Disabled:
		return otellog.SeverityUndefined
	}

	return otellog.SeverityUndefined
}

func withSpan(ctx context.Context, fields map[string]any) context.Context {
	traceHex, _ := fields[traceIDField].(string)
	spanHex, _ := fields[spanIDField].(string)

	traceID, err := trace.TraceIDFromHex(traceHex)
	if err != nil {
		return ctx
	}

	spanID, err := trace.SpanIDFromHex(spanHex)
	if err != nil {
		return ctx
	}

	return trace.ContextWithSpanContext(ctx, trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: trace.FlagsSampled,
	}))
}

func stringify(value any) string {
	if s, ok := value.(string); ok {
		return s
	}

	data, err := json.Marshal(value)
	if err != nil {
		return ""
	}

	return string(data)
}
