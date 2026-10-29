package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/trading/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/trading/internal/dto"
)

func (u *UseCase) ListIncomingRFQs(ctx context.Context, maker domain.Party) ([]dto.IncomingRFQ, error) {
	ctx, span := tracer.Start(ctx, "usecase ListIncomingRFQs")
	defer span.End()

	rfqs, err := u.postgres.ListMakerRFQs(ctx, maker.ID, time.Now().Add(-recentWindow))
	if err != nil {
		return nil, fmt.Errorf("postgres.ListMakerRFQs: %w", err)
	}

	quotes, err := u.quotesOf(ctx, rfqs)
	if err != nil {
		return nil, fmt.Errorf("u.quotesOf: %w", err)
	}

	out := make([]dto.IncomingRFQ, 0, len(rfqs))

	for _, rfq := range rfqs {
		incoming := dto.IncomingRFQ{RFQ: rfq}

		for _, q := range quotes[rfq.ID] {
			if q.Maker.ID == maker.ID {
				incoming.MyQuote = &q
			}
		}

		out = append(out, incoming)
	}

	return out, nil
}
