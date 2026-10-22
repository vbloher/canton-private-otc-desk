package usecase

import (
	"context"
	"errors"
	"fmt"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/dto"
)

// AuthenticateAPIKey resolves the API key of a bot (a house maker).
func (u *UseCase) AuthenticateAPIKey(
	ctx context.Context, input dto.AuthenticateAPIKeyInput,
) (dto.AuthenticateAPIKeyOutput, error) {
	ctx, span := tracer.Start(ctx, "usecase AuthenticateAPIKey")
	defer span.End()

	var output dto.AuthenticateAPIKeyOutput

	if input.APIKey == "" {
		return output, domain.ErrUnauthorized
	}

	user, err := u.postgres.GetUserByAPIKeyHash(ctx, domain.HashAPIKey(input.APIKey))
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return output, fmt.Errorf("unknown API key: %w", domain.ErrUnauthorized)
		}

		return output, fmt.Errorf("postgres.GetUserByAPIKeyHash: %w", err)
	}

	return dto.AuthenticateAPIKeyOutput{User: user}, nil
}
