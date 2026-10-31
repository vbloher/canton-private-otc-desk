package logger

import (
	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel/trace"
)

const (
	traceIDField = "trace_id"
	spanIDField  = "span_id"
)

type TraceHook struct{}

func (TraceHook) Run(e *zerolog.Event, _ zerolog.Level, _ string) {
	sc := trace.SpanContextFromContext(e.GetCtx())
	if !sc.IsSampled() {
		return
	}

	e.Str(traceIDField, sc.TraceID().String()).Str(spanIDField, sc.SpanID().String())
}
