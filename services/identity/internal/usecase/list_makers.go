package usecase

import (
	"context"
	"fmt"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/dto"
)

func (u *UseCase) ListMakers(ctx context.Context, input dto.ListMakersInput) (dto.ListMakersOutput, error) {
	ctx, span := tracer.Start(ctx, "usecase ListMakers")
	defer span.End()

	var output dto.ListMakersOutput

	makers, err := u.postgres.ListMakers(ctx)
	if err != nil {
		return output, fmt.Errorf("postgres.ListMakers: %w", err)
	}

	others := make([]domain.User, 0, len(makers))

	for _, maker := range makers {
		if maker.ID != input.CallerID {
			others = append(others, maker)
		}
	}

	return dto.ListMakersOutput{Makers: others}, nil
}
