package usecase

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/dto"
)

// Authenticate resolves a session token. A user read from the cache is at most a minute old;
// the cache is dropped on logout.
func (u *UseCase) Authenticate(ctx context.Context, input dto.AuthenticateInput) (dto.AuthenticateOutput, error) {
	ctx, span := tracer.Start(ctx, "usecase Authenticate")
	defer span.End()

	var output dto.AuthenticateOutput

	if input.Token == "" {
		return output, domain.ErrUnauthorized
	}

	hash := domain.HashSessionToken(input.Token)

	user, ok := u.redis.GetCachedSession(ctx, hash)
	if ok {
		return dto.AuthenticateOutput{User: user}, nil
	}

	user, err := u.postgres.GetUserBySession(ctx, hash, time.Now())
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return output, fmt.Errorf("unknown or expired session: %w", domain.ErrUnauthorized)
		}

		return output, fmt.Errorf("postgres.GetUserBySession: %w", err)
	}

	u.redis.CacheSession(ctx, hash, user, sessionCacheTTL)

	return dto.AuthenticateOutput{User: user}, nil
}
