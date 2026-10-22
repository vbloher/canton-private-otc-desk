package usecase

import (
	"context"
	"fmt"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/pkg/principal"
	"github.com/vbloher/canton-private-otc-desk/services/gateway/internal/domain"
)

// Authenticate resolves the caller from an API key or a session token.
// The API key wins when both are present, so a bot never falls back to a stale cookie.
func (u *UseCase) Authenticate(ctx context.Context, sessionToken, apiKey string) (principal.Principal, error) {
	ctx, span := tracer.Start(ctx, "usecase Authenticate")
	defer span.End()

	switch {
	case apiKey != "":
		p, err := u.identity.AuthenticateAPIKey(ctx, apiKey)
		if err != nil {
			return principal.Principal{}, fmt.Errorf("identity.AuthenticateAPIKey: %w", err)
		}

		return p, nil
	case sessionToken != "":
		p, err := u.identity.Authenticate(ctx, sessionToken)
		if err != nil {
			return principal.Principal{}, fmt.Errorf("identity.Authenticate: %w", err)
		}

		return p, nil
	default:
		return principal.Principal{}, domain.ErrUnauthorized
	}
}
