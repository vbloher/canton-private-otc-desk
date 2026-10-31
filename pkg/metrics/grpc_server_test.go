package metrics

import (
	"context"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	dto "github.com/prometheus/client_model/go"
)

func sampleCount(t *testing.T, o prometheus.Observer) uint64 {
	t.Helper()

	m, ok := o.(prometheus.Metric)
	require.True(t, ok)

	var out dto.Metric

	require.NoError(t, m.Write(&out))

	return out.GetHistogram().GetSampleCount()
}

func TestGRPCInterceptor_CountsOK(t *testing.T) {
	info := &grpc.UnaryServerInfo{FullMethod: "/ledger.v1.LedgerV1/TestOK"}

	resp, err := GRPCInterceptor(t.Context(), nil, info, func(context.Context, any) (any, error) { return "ok", nil })
	require.NoError(t, err)
	require.Equal(t, "ok", resp)

	require.InDelta(t, 1, testutil.ToFloat64(grpcRequests.WithLabelValues(info.FullMethod, "OK")), 0)
	require.Equal(t, uint64(1), sampleCount(t, grpcDuration.WithLabelValues(info.FullMethod)))
}

func TestGRPCInterceptor_CountsErrorCode(t *testing.T) {
	info := &grpc.UnaryServerInfo{FullMethod: "/ledger.v1.LedgerV1/TestUnavailable"}
	want := status.Error(codes.Unavailable, "x")

	_, err := GRPCInterceptor(t.Context(), nil, info, func(context.Context, any) (any, error) { return nil, want })
	require.ErrorIs(t, err, want)

	require.InDelta(t, 1, testutil.ToFloat64(grpcRequests.WithLabelValues(info.FullMethod, "Unavailable")), 0)
}
