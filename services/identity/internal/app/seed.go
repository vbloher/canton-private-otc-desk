package app

import (
	"context"
	"time"

	"github.com/rs/zerolog/log"
)

const seedRetryDelay = 2 * time.Second

type houseSeeder interface {
	SeedHouseMaker(ctx context.Context) error
}

func seedHouseMaker(ctx context.Context, seeder houseSeeder, retryDelay time.Duration) {
	for attempt := 1; ; attempt++ {
		err := seeder.SeedHouseMaker(ctx)
		if err == nil {
			log.Info().Int("attempt", attempt).Msg("app: the house maker is seeded")

			return
		}

		if ctx.Err() != nil {
			return
		}

		log.Warn().Err(err).Int("attempt", attempt).Str("retry_in", retryDelay.String()).
			Msg("app: seeding the house maker failed, will retry")

		select {
		case <-ctx.Done():
			return
		case <-time.After(retryDelay):
		}
	}
}
