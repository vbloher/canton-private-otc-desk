package usecase

import (
	"context"
	"errors"
	"fmt"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/dto"
)

// Logout ends every session the request names, once each: a client can present a cookie and a bearer
// token that differ, and the gateway prefers the bearer token, so ending only one of them could leave
// the session the client actually used alive. An empty or unknown token is not an error: the caller
// wanted to be signed out and is. A session that cannot be ended does not spare the others; the
// errors are returned together.
func (u *UseCase) Logout(ctx context.Context, input dto.LogoutInput) error {
	ctx, span := tracer.Start(ctx, "usecase Logout")
	defer span.End()

	var errs []error

	named := make(map[string]struct{}, len(input.Tokens))

	for _, token := range input.Tokens {
		if token == "" {
			continue
		}

		if _, again := named[token]; again {
			continue
		}

		named[token] = struct{}{}

		err := u.endSession(ctx, token)
		if err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

// endSession deletes the session of the token and, only once that worked, its cache entry: a session
// that is still valid stays cached.
func (u *UseCase) endSession(ctx context.Context, token string) error {
	hash := domain.HashSessionToken(token)

	err := u.postgres.DeleteSession(ctx, hash)
	if err != nil {
		return fmt.Errorf("postgres.DeleteSession: %w", err)
	}

	u.redis.DropCachedSession(ctx, hash)

	return nil
}
