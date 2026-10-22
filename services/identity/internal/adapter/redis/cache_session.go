package redis

import (
	"context"
	"encoding/json"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/domain"
)

// CacheSession is best effort: a session that is not cached is simply read from Postgres again.
// The wrapped key and the API key hash are not cached (see domain.User).
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
