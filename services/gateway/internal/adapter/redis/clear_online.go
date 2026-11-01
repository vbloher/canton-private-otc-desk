package redis

import (
	"context"
	"fmt"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/pkg/presence"
)

func (r *Redis) ClearOnline(ctx context.Context, partyID string) error {
	ctx, span := tracer.Start(ctx, "adapter redis ClearOnline")
	defer span.End()

	err := r.redis.Del(ctx, presence.Key(partyID)).Err()
	if err != nil {
		return fmt.Errorf("r.redis.Del: %w", err)
	}

	return nil
}
