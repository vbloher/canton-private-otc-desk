package usecase

import (
	"context"
	"fmt"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/dto"
)

func (u *UseCase) GetMe(ctx context.Context, input dto.GetMeInput) (dto.GetMeOutput, error) {
	ctx, span := tracer.Start(ctx, "usecase GetMe")
	defer span.End()

	var output dto.GetMeOutput

	user, err := u.postgres.GetUser(ctx, input.UserID)
	if err != nil {
		return output, fmt.Errorf("postgres.GetUser: %w", err)
	}

	return dto.GetMeOutput{User: user}, nil
}
