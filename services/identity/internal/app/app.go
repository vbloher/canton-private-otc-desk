package app

import (
	"context"
	"fmt"

	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"

	pb "github.com/vbloher/canton-private-otc-desk/gen/grpc/identity_v1"
	"github.com/vbloher/canton-private-otc-desk/pkg/grpcserver"
	"github.com/vbloher/canton-private-otc-desk/pkg/httpserver"
	"github.com/vbloher/canton-private-otc-desk/pkg/metrics"
	"github.com/vbloher/canton-private-otc-desk/pkg/migrate"
	pgpool "github.com/vbloher/canton-private-otc-desk/pkg/postgres"
	redislib "github.com/vbloher/canton-private-otc-desk/pkg/redis"
	"github.com/vbloher/canton-private-otc-desk/pkg/router"
	"github.com/vbloher/canton-private-otc-desk/pkg/transaction"
	"github.com/vbloher/canton-private-otc-desk/services/identity/config"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/adapter/ledger"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/adapter/postgres"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/adapter/redis"
	grpcv1 "github.com/vbloher/canton-private-otc-desk/services/identity/internal/controller/grpc/v1"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/controller/http"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/usecase"
	"github.com/vbloher/canton-private-otc-desk/services/identity/migration"
)

func Run(ctx context.Context, c config.Config) error { //nolint:funlen
	err := migrate.Up(c.Postgres.URL(), migration.FS)
	if err != nil {
		return fmt.Errorf("migrate.Up: %w", err)
	}

	pgPool, err := pgpool.New(ctx, c.Postgres)
	if err != nil {
		return fmt.Errorf("postgres.New: %w", err)
	}
	defer pgPool.Close()

	tx := transaction.New(pgPool)

	redisClient, err := redislib.New(ctx, c.Redis)
	if err != nil {
		return fmt.Errorf("redislib.New: %w", err)
	}
	defer redisClient.Close()

	ledgerClient, err := ledger.New(c.Ledger)
	if err != nil {
		return fmt.Errorf("ledger.New: %w", err)
	}
	defer ledgerClient.Close()

	uc := usecase.New(
		postgres.New(tx),
		redis.New(redisClient),
		ledgerClient,
		c.UseCase,
	)

	seedCtx, stopSeeding := context.WithCancel(ctx)
	seeded := make(chan struct{})

	go func() {
		defer close(seeded)

		seedHouseMaker(seedCtx, uc, seedRetryDelay)
	}()

	defer func() {
		stopSeeding()
		<-seeded
	}()

	grpcServer, err := grpcserver.New(ctx, c.GRPC, func(s *grpc.Server) {
		pb.RegisterIdentityV1Server(s, grpcv1.New(uc))
	})
	if err != nil {
		return fmt.Errorf("grpcserver.New: %w", err)
	}
	defer grpcServer.Close()

	r := router.New()

	err = http.IdentityRouter(r, uc, metrics.NewHTTPServer(), c.Router)
	if err != nil {
		return fmt.Errorf("http.IdentityRouter: %w", err)
	}

	httpServer, err := httpserver.New(ctx, r, c.HTTP)
	if err != nil {
		return fmt.Errorf("httpserver.New: %w", err)
	}
	defer httpServer.Close()

	log.Info().Msg("app: started")

	<-ctx.Done()

	log.Info().Msg("app: got signal to stop")

	return nil
}
