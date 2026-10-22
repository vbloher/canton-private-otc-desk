package usecase

import (
	"context"
	"fmt"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/dto"
)

// ResolveMakers returns the makers among the hints, in request order and each once. A hint that is
// unknown, or whose owner has maker mode off, is left out.
func (u *UseCase) ResolveMakers(ctx context.Context, input dto.ResolveMakersInput) (dto.ResolveMakersOutput, error) {
	ctx, span := tracer.Start(ctx, "usecase ResolveMakers")
	defer span.End()

	var output dto.ResolveMakersOutput

	if len(input.Hints) == 0 {
		return output, nil
	}

	makers, err := u.postgres.ListMakers(ctx)
	if err != nil {
		return output, fmt.Errorf("postgres.ListMakers: %w", err)
	}

	byHint := make(map[string]domain.User, len(makers))
	for _, maker := range makers {
		byHint[maker.Hint] = maker
	}

	resolved := make([]domain.User, 0, len(input.Hints))

	for _, hint := range input.Hints {
		maker, ok := byHint[hint]
		if !ok {
			continue
		}

		resolved = append(resolved, maker)

		delete(byHint, hint)
	}

	return dto.ResolveMakersOutput{Makers: resolved}, nil
}
