package usecase

import (
	"context"
	"fmt"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/trading/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/trading/internal/dto"
)

func (u *UseCase) GetDesk(ctx context.Context) dto.GetDeskOutput {
	_, span := tracer.Start(ctx, "usecase GetDesk")
	defer span.End()

	return dto.GetDeskOutput{FeeBps: u.config.FeeBps}
}

func (u *UseCase) desk(ctx context.Context) (domain.Desk, error) {
	desk, ok := u.redis.GetDesk(ctx)
	if ok {
		return desk, nil
	}

	desk, err := u.ledger.Desk(ctx)
	if err != nil {
		return domain.Desk{}, fmt.Errorf("ledger.Desk: %w", err)
	}

	u.redis.SetDesk(ctx, desk)

	return desk, nil
}
