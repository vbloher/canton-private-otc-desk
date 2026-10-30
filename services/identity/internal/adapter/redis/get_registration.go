package redis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"

	goredis "github.com/redis/go-redis/v9"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/domain"
)

func (r *Redis) GetRegistration(ctx context.Context, id uuid.UUID) (domain.Registration, error) {
	ctx, span := tracer.Start(ctx, "adapter redis GetRegistration")
	defer span.End()

	value, err := r.redis.Get(ctx, registrationPrefix+id.String()).Bytes()
	if err != nil {
		if errors.Is(err, goredis.Nil) {
			return domain.Registration{}, domain.ErrRegistrationExpired
		}

		return domain.Registration{}, fmt.Errorf("r.redis.Get: %w", err)
	}

	var reg domain.Registration

	err = json.Unmarshal(value, &reg)
	if err != nil {
		return domain.Registration{}, fmt.Errorf("json.Unmarshal: %w", err)
	}

	return reg, nil
}
