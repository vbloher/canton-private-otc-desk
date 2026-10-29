package redis

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/rs/zerolog/log"

	goredis "github.com/redis/go-redis/v9"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/domain"
)

func (r *Redis) GetCachedSession(ctx context.Context, tokenHash string) (domain.User, bool) {
	ctx, span := tracer.Start(ctx, "adapter redis GetCachedSession")
	defer span.End()

	value, err := r.redis.Get(ctx, sessionPrefix+tokenHash).Bytes()
	if err != nil {
		if !errors.Is(err, goredis.Nil) {
			log.Error().Err(err).Msg("redis: GetCachedSession: r.redis.Get")
		}

		return domain.User{}, false
	}

	var user domain.User

	err = json.Unmarshal(value, &user)
	if err != nil {
		log.Error().Err(err).Msg("redis: GetCachedSession: json.Unmarshal")

		return domain.User{}, false
	}

	return user, true
}
