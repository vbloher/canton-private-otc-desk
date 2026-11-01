package usecase

import (
	"context"
	"fmt"
	"slices"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/dto"
)

func (u *UseCase) ListMakers(ctx context.Context, input dto.ListMakersInput) (dto.ListMakersOutput, error) {
	ctx, span := tracer.Start(ctx, "usecase ListMakers")
	defer span.End()

	var output dto.ListMakersOutput

	makers, err := u.onlineMakers(ctx)
	if err != nil {
		return output, err
	}

	others := make([]domain.User, 0, len(makers))

	for _, maker := range makers {
		if maker.ID != input.CallerID {
			others = append(others, maker)
		}
	}

	return dto.ListMakersOutput{Makers: others}, nil
}

func (u *UseCase) onlineMakers(ctx context.Context) ([]domain.User, error) {
	makers, err := u.postgres.ListMakers(ctx)
	if err != nil {
		return nil, fmt.Errorf("postgres.ListMakers: %w", err)
	}

	if len(makers) == 0 {
		return makers, nil
	}

	partyIDs := make([]string, len(makers))
	for i, maker := range makers {
		partyIDs[i] = maker.PartyID
	}

	online, err := u.redis.Online(ctx, partyIDs)
	if err != nil {
		return nil, fmt.Errorf("redis.Online: %w", err)
	}

	return slices.DeleteFunc(makers, func(maker domain.User) bool { return !online[maker.PartyID] }), nil
}
