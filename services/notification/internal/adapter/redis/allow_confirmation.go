package redis

import (
	"context"
	"fmt"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/notification/internal/domain"
)

func (r *Redis) AllowConfirmation(ctx context.Context, partyID, email string) (bool, error) {
	ctx, span := tracer.Start(ctx, "adapter redis AllowConfirmation")
	defer span.End()

	free, err := r.redis.SetNX(ctx, cooldownKeyPrefix+partyID, 1, r.limits.PartyCooldown).Result()
	if err != nil {
		return false, fmt.Errorf("r.redis.SetNX: %w", err)
	}

	if !free {
		return false, nil
	}

	counters := []struct {
		key   string
		limit int
	}{
		{partyKeyPrefix + partyID, r.limits.PartyPerDay},
		{emailKeyPrefix + domain.HashToken(domain.CanonicalEmail(email)), r.limits.EmailPerDay},
		{totalKey, r.limits.TotalPerDay},
	}

	for _, counter := range counters {
		count, err := countInWindow.Run(ctx, r.redis, []string{counter.key}, day.Milliseconds()).Int()
		if err != nil {
			return false, fmt.Errorf("countInWindow.Run: %w", err)
		}

		if count > counter.limit {
			return false, nil
		}
	}

	return true, nil
}
