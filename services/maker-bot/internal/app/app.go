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

// Run starts the bot; it has no state of its own and keeps polling while the gateway is down.
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

	log.Info().Int("makers", len(c.UseCase.Makers)).Msg("app: started")

	<-ctx.Done()

	log.Info().Msg("app: got signal to stop")

	return nil
}
