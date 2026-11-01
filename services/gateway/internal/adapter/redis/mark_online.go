package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/pkg/presence"
)

func (r *Redis) MarkOnline(ctx context.Context, partyIDs []string, ttl time.Duration) error {
	ctx, span := tracer.Start(ctx, "adapter redis MarkOnline")
	defer span.End()

	if len(partyIDs) == 0 {
		return nil
	}

	pipe := r.redis.Pipeline()

	for _, partyID := range partyIDs {
		pipe.Set(ctx, presence.Key(partyID), "1", ttl)
	}

	_, err := pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("pipe.Exec: %w", err)
	}

	return nil
}
