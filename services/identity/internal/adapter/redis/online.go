package redis

import (
	"context"
	"fmt"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/pkg/presence"
)

func (r *Redis) Online(ctx context.Context, partyIDs []string) (map[string]bool, error) {
	ctx, span := tracer.Start(ctx, "adapter redis Online")
	defer span.End()

	online := make(map[string]bool, len(partyIDs))

	if len(partyIDs) == 0 {
		return online, nil
	}

	keys := make([]string, len(partyIDs))
	for i, partyID := range partyIDs {
		keys[i] = presence.Key(partyID)
	}

	values, err := r.redis.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, fmt.Errorf("r.redis.MGet: %w", err)
	}

	for i, value := range values {
		if value != nil {
			online[partyIDs[i]] = true
		}
	}

	return online, nil
}
