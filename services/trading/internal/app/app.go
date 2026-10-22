package app

import (
	"context"
	"fmt"

	"github.com/rs/zerolog/log"

	"github.com/vbloher/canton-private-otc-desk/pkg/events"
	"github.com/vbloher/canton-private-otc-desk/pkg/httpserver"
	"github.com/vbloher/canton-private-otc-desk/pkg/metrics"
	"github.com/vbloher/canton-private-otc-desk/pkg/migrate"
	pgpool "github.com/vbloher/canton-private-otc-desk/pkg/postgres"
	redislib "github.com/vbloher/canton-private-otc-desk/pkg/redis"
	"github.com/vbloher/canton-private-otc-desk/pkg/router"
	"github.com/vbloher/canton-private-otc-desk/pkg/transaction"
	"github.com/vbloher/canton-private-otc-desk/services/trading/config"
	"github.com/vbloher/canton-private-otc-desk/services/trading/internal/adapter/identity"
	"github.com/vbloher/canton-private-otc-desk/services/trading/internal/adapter/ledger"
	"github.com/vbloher/canton-private-otc-desk/services/trading/internal/adapter/postgres"
	"github.com/vbloher/canton-private-otc-desk/services/trading/internal/adapter/redis"
	"github.com/vbloher/canton-private-otc-desk/services/trading/internal/controller/http"
	"github.com/vbloher/canton-private-otc-desk/services/trading/internal/controller/worker"
	"github.com/vbloher/canton-private-otc-desk/services/trading/internal/usecase"
	"github.com/vbloher/canton-private-otc-desk/services/trading/migration"
)

// Run starts the service. The ledger and identity clients connect lazily: while either is down
// the service runs and the routes that need it answer 502.
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

	identityClient, err := identity.New(c.Identity)
	if err != nil {
		return fmt.Errorf("identity.New: %w", err)
	}
	defer identityClient.Close()

	uc := usecase.New(
		c.UseCase,
		tx,
		postgres.New(tx),
		ledgerClient,
		identityClient,
		redis.New(redisClient),
		events.NewPublisher(redisClient),
	)

	settlementWorker, err := worker.New(c.Worker, uc)
	if err != nil {
		return fmt.Errorf("worker.New: %w", err)
	}
	defer settlementWorker.Close()

	r := router.New()

	err = http.TradingRouter(r, uc, metrics.NewHTTPServer())
	if err != nil {
		return fmt.Errorf("http.TradingRouter: %w", err)
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
