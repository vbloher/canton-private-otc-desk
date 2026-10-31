package grpcserver

import (
	"context"
	"fmt"
	"net"

	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/reflection"

	"github.com/vbloher/canton-private-otc-desk/pkg/logger"
	"github.com/vbloher/canton-private-otc-desk/pkg/metrics"
)

type Config struct {
	Host string `env:"GRPC_HOST"`
	Port string `env:"GRPC_PORT"`
}

type Server struct {
	server *grpc.Server
}

func New(ctx context.Context, c Config, register func(s *grpc.Server)) (*Server, error) {
	s := grpc.NewServer(
		grpc.Creds(insecure.NewCredentials()),
		grpc.StatsHandler(otelgrpc.NewServerHandler()),
		grpc.ChainUnaryInterceptor(logger.Interceptor, metrics.GRPCInterceptor),
	)

	reflection.Register(s)
	register(s)

	var lc net.ListenConfig

	conn, err := lc.Listen(ctx, "tcp", net.JoinHostPort(c.Host, c.Port))
	if err != nil {
		return nil, fmt.Errorf("net.Listen: %w", err)
	}

	go func() {
		err := s.Serve(conn)
		if err != nil {
			log.Error().Err(err).Msg("grpc server: Serve")
		}
	}()

	log.Info().Msg("grpc server: started on port: " + c.Port)

	return &Server{server: s}, nil
}

func (s *Server) Close() {
	s.server.GracefulStop()

	log.Info().Msg("grpc server: closed")
}
