package app

import (
	"context"
	"fmt"

	"github.com/rs/zerolog/log"

	"github.com/vbloher/canton-private-otc-desk/services/maker-bot/config"
	"github.com/vbloher/canton-private-otc-desk/services/maker-bot/internal/adapter/gateway"
	"github.com/vbloher/canton-private-otc-desk/services/maker-bot/internal/controller/worker"
	"github.com/vbloher/canton-private-otc-desk/services/maker-bot/internal/usecase"
)

func Run(ctx context.Context, c config.Config) error {
	gw, err := gateway.New(c.Gateway)
	if err != nil {
		return fmt.Errorf("gateway.New: %w", err)
	}

	bot, err := worker.New(c.Worker, usecase.New(c.UseCase, gw))
	if err != nil {
		return fmt.Errorf("worker.New: %w", err)
	}
	defer bot.Close()

	log.Info().Int64("spread_bps", int64(c.UseCase.Spread)).Msg("app: started")

	<-ctx.Done()

	log.Info().Msg("app: got signal to stop")

	return nil
}
