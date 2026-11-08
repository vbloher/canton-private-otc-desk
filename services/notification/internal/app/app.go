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
	"github.com/vbloher/canton-private-otc-desk/services/notification/config"
	"github.com/vbloher/canton-private-otc-desk/services/notification/internal/adapter/logmail"
	"github.com/vbloher/canton-private-otc-desk/services/notification/internal/adapter/postgres"
	"github.com/vbloher/canton-private-otc-desk/services/notification/internal/adapter/redis"
	"github.com/vbloher/canton-private-otc-desk/services/notification/internal/adapter/resend"
	"github.com/vbloher/canton-private-otc-desk/services/notification/internal/controller/http"
	"github.com/vbloher/canton-private-otc-desk/services/notification/internal/controller/kafka"
	"github.com/vbloher/canton-private-otc-desk/services/notification/internal/controller/worker"
	"github.com/vbloher/canton-private-otc-desk/services/notification/internal/usecase"
	"github.com/vbloher/canton-private-otc-desk/services/notification/migration"
)

type mailer interface {
	usecase.Sender
	usecase.Contacts
}

func Run(ctx context.Context, c config.Config) error { //nolint:funlen
	err := pgpool.EnsureDatabase(ctx, c.Postgres)
	if err != nil {
		return fmt.Errorf("postgres.EnsureDatabase: %w", err)
	}

	err = migrate.Up(c.Postgres.URL(), migration.FS)
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

	mail, err := newMailer(c)
	if err != nil {
		return fmt.Errorf("newMailer: %w", err)
	}

	uc := usecase.New(c.UseCase, tx, postgres.New(tx), redis.New(redisClient, redis.LimitsFrom(c.Limits)), mail, mail,
		events.NewPublisher(redisClient))

	consumer, err := kafka.New(c.Kafka, c.Consumer, uc)
	if err != nil {
		return fmt.Errorf("kafka.New: %w", err)
	}
	defer consumer.Close()

	emailWorker, err := worker.New(c.Worker, uc)
	if err != nil {
		return fmt.Errorf("worker.New: %w", err)
	}
	defer emailWorker.Close()

	r := router.New()

	err = http.NotificationRouter(r, uc, metrics.NewHTTPServer())
	if err != nil {
		return fmt.Errorf("http.NotificationRouter: %w", err)
	}

	httpServer, err := httpserver.New(ctx, r, c.HTTP)
	if err != nil {
		return fmt.Errorf("httpserver.New: %w", err)
	}
	defer httpServer.Close()

	log.Info().Bool("email", c.UseCase.Enabled).Msg("app: started")

	<-ctx.Done()

	log.Info().Msg("app: got signal to stop")

	return nil
}

func newMailer(c config.Config) (mailer, error) {
	if c.LogEmails || c.Email.APIKey == "" {
		return logmail.New(), nil
	}

	client, err := resend.New(c.Email)
	if err != nil {
		return nil, fmt.Errorf("resend.New: %w", err)
	}

	return client, nil
}
