package redis

import (
	"context"

	"github.com/rs/zerolog/log"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
)

func (r *Redis) ReleaseFaucet(ctx context.Context, party string) {
	ctx, span := tracer.Start(ctx, "adapter redis ReleaseFaucet")
	defer span.End()

	err := r.redis.Del(ctx, faucetKeyPrefix+party).Err()
	if err != nil {
		log.Error().Err(err).Msg("redis: ReleaseFaucet: r.redis.Del")
	}
}
