package app

import (
	"context"
	"fmt"

	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"

	pb "github.com/vbloher/canton-private-otc-desk/gen/grpc/ledger_v1"
	"github.com/vbloher/canton-private-otc-desk/pkg/events"
	"github.com/vbloher/canton-private-otc-desk/pkg/grpcserver"
	"github.com/vbloher/canton-private-otc-desk/pkg/httpserver"
	"github.com/vbloher/canton-private-otc-desk/pkg/metrics"
	"github.com/vbloher/canton-private-otc-desk/pkg/migrate"
	pgpool "github.com/vbloher/canton-private-otc-desk/pkg/postgres"
	redislib "github.com/vbloher/canton-private-otc-desk/pkg/redis"
	"github.com/vbloher/canton-private-otc-desk/pkg/router"
	"github.com/vbloher/canton-private-otc-desk/pkg/transaction"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/config"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/adapter/canton"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/adapter/postgres"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/adapter/redis"
	grpcv1 "github.com/vbloher/canton-private-otc-desk/services/ledger/internal/controller/grpc/v1"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/controller/http"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/controller/worker"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/usecase"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/migration"
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

	uc := usecase.New(
		c.Ledger,
		tx,
		postgres.New(tx),
		redis.New(redisClient),
		canton.New(c.Canton),
		events.NewPublisher(redisClient),
	)

	// Nothing works without the operator and the house parties: wait for the ledger, or
	// exit and let the supervisor restart us.
	err = uc.Init(ctx)
	if err != nil {
		return fmt.Errorf("uc.Init: %w", err)
	}

	expireWorker := worker.New(uc)
	defer expireWorker.Close()

	grpcServer, err := grpcserver.New(ctx, c.GRPC, func(s *grpc.Server) {
		pb.RegisterLedgerV1Server(s, grpcv1.New(uc))
	})
	if err != nil {
		return fmt.Errorf("grpcserver.New: %w", err)
	}
	defer grpcServer.Close()

	r := router.New()

	err = http.WalletRouter(r, uc, metrics.NewHTTPServer())
	if err != nil {
		return fmt.Errorf("http.WalletRouter: %w", err)
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
