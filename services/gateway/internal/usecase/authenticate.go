package usecase

import (
	"context"
	"fmt"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/pkg/principal"
	"github.com/vbloher/canton-private-otc-desk/services/gateway/internal/domain"
)

func (u *UseCase) Authenticate(ctx context.Context, sessionToken string) (principal.Principal, error) {
	ctx, span := tracer.Start(ctx, "usecase Authenticate")
	defer span.End()

	if sessionToken == "" {
		return principal.Principal{}, domain.ErrUnauthorized
	}

	p, err := u.identity.Authenticate(ctx, sessionToken)
	if err != nil {
		return principal.Principal{}, fmt.Errorf("identity.Authenticate: %w", err)
	}

	return p, nil
}
