package usecase

import (
	"context"
	"fmt"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/notification/internal/dto"
)

func (u *UseCase) GetProfile(ctx context.Context, input dto.GetProfileInput) (dto.ProfileOutput, error) {
	ctx, span := tracer.Start(ctx, "usecase GetProfile")
	defer span.End()

	found, err := u.postgres.GetProfile(ctx, input.PartyID)

	profile, err := orEmpty(input.PartyID, found, err)
	if err != nil {
		return dto.ProfileOutput{}, fmt.Errorf("postgres.GetProfile: %w", err)
	}

	return dto.ProfileOutput{Profile: profile, EmailEnabled: u.config.Enabled}, nil
}
