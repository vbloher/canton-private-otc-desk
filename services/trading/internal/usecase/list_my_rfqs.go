package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/trading/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/trading/internal/dto"
)

func (u *UseCase) ListMyRFQs(ctx context.Context, taker domain.Party) ([]dto.RFQWithQuotes, error) {
	ctx, span := tracer.Start(ctx, "usecase ListMyRFQs")
	defer span.End()

	rfqs, err := u.postgres.ListTakerRFQs(ctx, taker.ID, time.Now().Add(-recentWindow))
	if err != nil {
		return nil, fmt.Errorf("postgres.ListTakerRFQs: %w", err)
	}

	quotes, err := u.quotesOf(ctx, rfqs)
	if err != nil {
		return nil, fmt.Errorf("u.quotesOf: %w", err)
	}

	out := make([]dto.RFQWithQuotes, 0, len(rfqs))

	for _, rfq := range rfqs {
		visible := make([]domain.Quote, 0, len(quotes[rfq.ID]))

		for _, q := range quotes[rfq.ID] {
			if q.VisibleToTaker() {
				visible = append(visible, q)
			}
		}

		out = append(out, dto.RFQWithQuotes{RFQ: rfq, Quotes: visible})
	}

	return out, nil
}

func (u *UseCase) quotesOf(ctx context.Context, rfqs []domain.RFQ) (map[uuid.UUID][]domain.Quote, error) {
	if len(rfqs) == 0 {
		return map[uuid.UUID][]domain.Quote{}, nil
	}

	ids := make([]uuid.UUID, 0, len(rfqs))
	for _, rfq := range rfqs {
		ids = append(ids, rfq.ID)
	}

	quotes, err := u.postgres.ListQuotes(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("postgres.ListQuotes: %w", err)
	}

	byRFQ := make(map[uuid.UUID][]domain.Quote, len(rfqs))
	for _, q := range quotes {
		byRFQ[q.RFQID] = append(byRFQ[q.RFQID], q)
	}

	return byRFQ, nil
}
