package redis

import (
	"context"
	"errors"

	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog/log"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
)

func (r *Redis) GetSynchronizerID(ctx context.Context) (string, bool) {
	ctx, span := tracer.Start(ctx, "adapter redis GetSynchronizerID")
	defer span.End()

	id, err := r.redis.Get(ctx, synchronizerKey).Result()
	if err != nil {
		if !errors.Is(err, redis.Nil) {
			log.Error().Err(err).Msg("redis: GetSynchronizerID: r.redis.Get")
		}

		return "", false
	}

	return id, id != ""
}
