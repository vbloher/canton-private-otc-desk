package logger_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"

	"github.com/vbloher/canton-private-otc-desk/pkg/logger"
)

func TestInterceptor_LogsTraceID(t *testing.T) {
	var buf bytes.Buffer

	prev := log.Logger
	log.Logger = zerolog.New(&buf).Hook(logger.TraceHook{})

	t.Cleanup(func() { log.Logger = prev })

	handler := func(context.Context, any) (any, error) { return nil, nil }

	_, err := logger.Interceptor(spanContext(t, trace.FlagsSampled), nil, &grpc.UnaryServerInfo{FullMethod: "/ledger.v1.LedgerV1/Settle"}, handler)
	require.NoError(t, err)

	fields := decode(t, &buf)
	require.Equal(t, traceID, fields["trace_id"])
	require.Equal(t, "/ledger.v1.LedgerV1/Settle", fields["grpc_method"])
}
