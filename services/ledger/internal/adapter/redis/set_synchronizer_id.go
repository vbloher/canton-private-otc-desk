package redis

import (
	"context"

	"github.com/rs/zerolog/log"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
)

func (r *Redis) SetSynchronizerID(ctx context.Context, id string) {
	ctx, span := tracer.Start(ctx, "adapter redis SetSynchronizerID")
	defer span.End()

	err := r.redis.Set(ctx, synchronizerKey, id, ttl).Err()
	if err != nil {
		log.Error().Err(err).Msg("redis: SetSynchronizerID: r.redis.Set")
	}
}
