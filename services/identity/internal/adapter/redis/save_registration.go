package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/domain"
)

func (r *Redis) SaveRegistration(ctx context.Context, reg domain.Registration, ttl time.Duration) error {
	ctx, span := tracer.Start(ctx, "adapter redis SaveRegistration")
	defer span.End()

	value, err := json.Marshal(reg)
	if err != nil {
		return fmt.Errorf("json.Marshal: %w", err)
	}

	err = r.redis.Set(ctx, registrationPrefix+reg.ID.String(), value, ttl).Err()
	if err != nil {
		return fmt.Errorf("r.redis.Set: %w", err)
	}

	return nil
}
