package usecase

import (
	"context"
	"fmt"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/dto"
)

func (u *UseCase) Logout(ctx context.Context, input dto.LogoutInput) error {
	ctx, span := tracer.Start(ctx, "usecase Logout")
	defer span.End()

	if input.Token == "" {
		return nil
	}

	hash := domain.HashSessionToken(input.Token)

	err := u.postgres.DeleteSession(ctx, hash)
	if err != nil {
		return fmt.Errorf("postgres.DeleteSession: %w", err)
	}

	u.redis.DropCachedSession(ctx, hash)

	return nil
}
