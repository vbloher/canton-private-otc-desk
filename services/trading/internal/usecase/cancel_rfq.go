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

func (u *UseCase) CancelRFQ(ctx context.Context, input dto.CancelRFQInput) error {
	ctx, span := tracer.Start(ctx, "usecase CancelRFQ")
	defer span.End()

	var (
		rfq       domain.RFQ
		cancelled []domain.Quote
	)

	err := u.tx.Wrap(ctx, func(ctx context.Context) error {
		var err error

		rfq, err = u.postgres.GetRFQForUpdate(ctx, input.RFQID)
		if err != nil {
			return fmt.Errorf("postgres.GetRFQForUpdate: %w", err)
		}

		if rfq.TakerParty != input.Taker.ID {
			return domain.ErrForbidden
		}

		if rfq.Status != domain.RFQOpen {
			return domain.ErrRFQClosed
		}

		quotes, err := u.postgres.ListQuotes(ctx, []uuid.UUID{rfq.ID})
		if err != nil {
			return fmt.Errorf("postgres.ListQuotes: %w", err)
		}

		if settling(quotes) {
			return domain.Errorf(domain.ErrRFQClosed, "a trade on it is settling")
		}

		rfq, cancelled, err = u.closeRFQ(ctx, rfq, quotes, domain.RFQCancelled, domain.QuoteCancelled, time.Now())
		if err != nil {
			return fmt.Errorf("u.closeRFQ: %w", err)
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("transaction.Wrap: %w", err)
	}

	u.closed(ctx, rfq, cancelled)

	return nil
}
