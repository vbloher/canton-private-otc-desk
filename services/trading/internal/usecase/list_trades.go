package usecase

import (
	"context"
	"fmt"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/trading/internal/domain"
)

func (u *UseCase) ListTrades(ctx context.Context, party domain.Party) ([]domain.Trade, error) {
	ctx, span := tracer.Start(ctx, "usecase ListTrades")
	defer span.End()

	trades, err := u.postgres.ListTrades(ctx, party.ID, listLimit)
	if err != nil {
		return nil, fmt.Errorf("postgres.ListTrades: %w", err)
	}

	return trades, nil
}
