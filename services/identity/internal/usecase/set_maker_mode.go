package usecase

import (
	"context"
	"fmt"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/dto"
)

func (u *UseCase) SetMakerMode(ctx context.Context, input dto.SetMakerModeInput) (dto.SetMakerModeOutput, error) {
	ctx, span := tracer.Start(ctx, "usecase SetMakerMode")
	defer span.End()

	var output dto.SetMakerModeOutput

	user, err := u.postgres.SetMakerMode(ctx, input.UserID, input.Enabled)
	if err != nil {
		return output, fmt.Errorf("postgres.SetMakerMode: %w", err)
	}

	return dto.SetMakerModeOutput{User: user}, nil
}
