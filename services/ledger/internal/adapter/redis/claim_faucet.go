package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
)

func (r *Redis) ClaimFaucet(ctx context.Context, party string, cooldown time.Duration) (bool, error) {
	ctx, span := tracer.Start(ctx, "adapter redis ClaimFaucet")
	defer span.End()

	claimed, err := r.redis.SetNX(ctx, faucetKeyPrefix+party, time.Now().UTC().Format(time.RFC3339), cooldown).Result()
	if err != nil {
		return false, fmt.Errorf("r.redis.SetNX: %w", err)
	}

	return claimed, nil
}
