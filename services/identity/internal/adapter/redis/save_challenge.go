package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
)

// SaveChallenge remembers which wallet a challenge was issued to.
func (r *Redis) SaveChallenge(ctx context.Context, challenge string, userID uuid.UUID, ttl time.Duration) error {
	ctx, span := tracer.Start(ctx, "adapter redis SaveChallenge")
	defer span.End()

	err := r.redis.Set(ctx, challengePrefix+challenge, userID.String(), ttl).Err()
	if err != nil {
		return fmt.Errorf("r.redis.Set: %w", err)
	}

	return nil
}
