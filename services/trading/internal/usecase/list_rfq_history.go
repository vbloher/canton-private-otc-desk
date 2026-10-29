package usecase

import (
	"context"
	"fmt"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/trading/internal/domain"
)

func (u *UseCase) ListRFQHistory(ctx context.Context, taker domain.Party) ([]domain.RFQ, error) {
	ctx, span := tracer.Start(ctx, "usecase ListRFQHistory")
	defer span.End()

	rfqs, err := u.postgres.ListRFQHistory(ctx, taker.ID, listLimit)
	if err != nil {
		return nil, fmt.Errorf("postgres.ListRFQHistory: %w", err)
	}

	return rfqs, nil
}
