package redis

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	goredis "github.com/redis/go-redis/v9"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/domain"
)

func (r *Redis) TakeChallenge(ctx context.Context, challenge string) (uuid.UUID, error) {
	ctx, span := tracer.Start(ctx, "adapter redis TakeChallenge")
	defer span.End()

	value, err := r.redis.GetDel(ctx, challengePrefix+challenge).Result()
	if err != nil {
		if errors.Is(err, goredis.Nil) {
			return uuid.Nil, fmt.Errorf("unknown, used or expired challenge: %w", domain.ErrUnauthorized)
		}

		return uuid.Nil, fmt.Errorf("r.redis.GetDel: %w", err)
	}

	userID, err := uuid.Parse(value)
	if err != nil {
		return uuid.Nil, fmt.Errorf("uuid.Parse: %w", err)
	}

	return userID, nil
}
