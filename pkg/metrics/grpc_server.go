package metrics

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
)

//nolint:gochecknoglobals
var (
	grpcRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "grpc_server_requests_total",
		Help: "Total number of gRPC requests",
	}, []string{labelMethod, "code"})

	grpcDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "grpc_server_request_duration_seconds",
		Help:    "Duration of gRPC requests in seconds",
		Buckets: buckets,
	}, []string{labelMethod})
)

func GRPCInterceptor(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	start := time.Now()

	resp, err := handler(ctx, req)

	grpcDuration.WithLabelValues(info.FullMethod).Observe(time.Since(start).Seconds())
	grpcRequests.WithLabelValues(info.FullMethod, status.Code(err).String()).Inc()

	return resp, err
}
