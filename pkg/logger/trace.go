package logger

import (
	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel/trace"
)

type TraceHook struct{}

func (TraceHook) Run(e *zerolog.Event, _ zerolog.Level, _ string) {
	sc := trace.SpanContextFromContext(e.GetCtx())
	if !sc.IsSampled() {
		return
	}

	e.Str("trace_id", sc.TraceID().String()).Str("span_id", sc.SpanID().String())
}
