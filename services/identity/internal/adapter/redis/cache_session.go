package redis

import (
	"context"
	"encoding/json"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/domain"
)

func (r *Redis) CacheSession(ctx context.Context, tokenHash string, user domain.User, ttl time.Duration) {
	ctx, span := tracer.Start(ctx, "adapter redis CacheSession")
	defer span.End()

	value, err := json.Marshal(user)
	if err != nil {
		log.Error().Err(err).Msg("redis: CacheSession: json.Marshal")

		return
	}

	err = r.redis.Set(ctx, sessionPrefix+tokenHash, value, ttl).Err()
	if err != nil {
		log.Error().Err(err).Msg("redis: CacheSession: r.redis.Set")
	}
}
