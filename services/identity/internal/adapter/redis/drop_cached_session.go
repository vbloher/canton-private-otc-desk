package redis

import (
	"context"

	"github.com/rs/zerolog/log"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
)

func (r *Redis) DropCachedSession(ctx context.Context, tokenHash string) {
	ctx, span := tracer.Start(ctx, "adapter redis DropCachedSession")
	defer span.End()

	err := r.redis.Del(ctx, sessionPrefix+tokenHash).Err()
	if err != nil {
		log.Error().Err(err).Msg("redis: DropCachedSession: r.redis.Del")
	}
}
