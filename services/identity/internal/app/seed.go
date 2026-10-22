package app

import (
	"context"
	"time"

	"github.com/rs/zerolog/log"
)

// seedRetryDelay is the pause between two attempts to seed the house makers.
const seedRetryDelay = 2 * time.Second

type houseSeeder interface {
	SeedHouseMakers(ctx context.Context) error
}

// seedHouseMakers registers the house makers and, while the ledger cannot tell their party ids yet,
// tries again every retryDelay until it works or ctx ends. It runs beside the servers, which do not
// wait for it: identity answers (house makers just cannot sign in) while the ledger is down.
func seedHouseMakers(ctx context.Context, seeder houseSeeder, retryDelay time.Duration) {
	for attempt := 1; ; attempt++ {
		err := seeder.SeedHouseMakers(ctx)
		if err == nil {
			log.Info().Int("attempt", attempt).Msg("app: house makers are seeded")

			return
		}

		if ctx.Err() != nil {
			return
		}

		log.Warn().Err(err).Int("attempt", attempt).Str("retry_in", retryDelay.String()).
			Msg("app: seeding the house makers failed, will retry")

		select {
		case <-ctx.Done():
			return
		case <-time.After(retryDelay):
		}
	}
}
