package usecase

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/vbloher/canton-private-otc-desk/pkg/events"
	"github.com/vbloher/canton-private-otc-desk/services/trading/internal/domain"
)

func (u *UseCase) lockQuote(ctx context.Context, rfqID, quoteID uuid.UUID) (domain.RFQ, domain.Quote, error) {
	rfq, err := u.postgres.GetRFQForUpdate(ctx, rfqID)
	if err != nil {
		return domain.RFQ{}, domain.Quote{}, fmt.Errorf("postgres.GetRFQForUpdate: %w", err)
	}

	quote, err := u.postgres.GetQuoteForUpdate(ctx, quoteID)
	if err != nil {
		return domain.RFQ{}, domain.Quote{}, fmt.Errorf("postgres.GetQuoteForUpdate: %w", err)
	}

	return rfq, quote, nil
}

func (u *UseCase) transition(
	ctx context.Context, q domain.Quote, change func(domain.Quote) (domain.Quote, bool),
) (domain.RFQ, domain.Quote, bool, error) {
	var (
		rfq   domain.RFQ
		quote domain.Quote
		done  bool
	)

	err := u.tx.Wrap(ctx, func(ctx context.Context) error {
		var err error

		rfq, quote, err = u.lockQuote(ctx, q.RFQID, q.ID)
		if err != nil {
			return fmt.Errorf("u.lockQuote: %w", err)
		}

		changed, ok := change(quote)
		if !ok {
			return nil
		}

		err = u.postgres.UpdateQuote(ctx, changed)
		if err != nil {
			return fmt.Errorf("postgres.UpdateQuote: %w", err)
		}

		quote, done = changed, true

		return nil
	})
	if err != nil {
		return domain.RFQ{}, domain.Quote{}, false, fmt.Errorf("transaction.Wrap: %w", err)
	}

	return rfq, quote, done, nil
}

func (u *UseCase) closeRFQ(
	ctx context.Context, rfq domain.RFQ, quotes []domain.Quote,
	status domain.RFQStatus, quoteStatus domain.QuoteStatus, now time.Time,
) (domain.RFQ, []domain.Quote, error) {
	rfq = rfq.Close(status, now)

	err := u.postgres.UpdateRFQ(ctx, rfq)
	if err != nil {
		return domain.RFQ{}, nil, fmt.Errorf("postgres.UpdateRFQ: %w", err)
	}

	closed := make([]domain.Quote, 0, len(quotes))

	for _, q := range quotes {
		if !q.Status.Live() {
			continue
		}

		q = q.WithStatus(quoteStatus)

		err = u.postgres.UpdateQuote(ctx, q)
		if err != nil {
			return domain.RFQ{}, nil, fmt.Errorf("postgres.UpdateQuote: %w", err)
		}

		closed = append(closed, q)
	}

	return rfq, closed, nil
}

func settling(quotes []domain.Quote) bool {
	return slices.ContainsFunc(quotes, func(q domain.Quote) bool { return q.Status == domain.QuoteAccepted })
}

func (u *UseCase) closed(ctx context.Context, rfq domain.RFQ, quotes []domain.Quote) {
	for _, q := range quotes {
		u.release(ctx, rfq.TakerParty, q)
	}

	u.publishRFQ(ctx, events.RFQUpdated, rfq)
}

func (u *UseCase) release(ctx context.Context, taker string, q domain.Quote) {
	u.cancelSettlement(ctx, q.ID)
	u.publishQuote(ctx, taker, q)
}

func (u *UseCase) cancelSettlement(ctx context.Context, settlementID uuid.UUID) {
	err := u.ledger.CancelSettlement(context.WithoutCancel(ctx), settlementID)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Stringer("settlement", settlementID).Msg("usecase: ledger.CancelSettlement")
	}
}

func (u *UseCase) failTrade(ctx context.Context, trade domain.Trade, reason string) error {
	var (
		quote  domain.Quote
		failed bool
	)

	err := u.tx.Wrap(ctx, func(ctx context.Context) error {
		var err error

		_, quote, err = u.lockQuote(ctx, trade.RFQID, trade.QuoteID)
		if err != nil {
			return fmt.Errorf("u.lockQuote: %w", err)
		}

		if quote.Status != domain.QuoteAccepted {
			return nil
		}

		trade = trade.Failed(reason)

		err = u.postgres.UpdateTrade(ctx, trade)
		if err != nil {
			return fmt.Errorf("postgres.UpdateTrade: %w", err)
		}

		quote = quote.WithStatus(domain.QuoteFailed)

		err = u.postgres.UpdateQuote(ctx, quote)
		if err != nil {
			return fmt.Errorf("postgres.UpdateQuote: %w", err)
		}

		failed = true

		return nil
	})
	if err != nil {
		return fmt.Errorf("transaction.Wrap: %w", err)
	}

	if failed {
		u.cancelSettlement(ctx, trade.QuoteID)
		u.publishTrade(ctx, trade)
		u.publishQuote(ctx, trade.Taker.ID, quote)
	}

	return nil
}

func describe(rfq domain.RFQ, quote domain.Quote, forTaker bool) string {
	verb := "Sell"
	if rfq.Direction.TakerBuys() == forTaker {
		verb = "Buy"
	}

	return fmt.Sprintf("%s %s %s at %s %s", verb, rfq.Qty, rfq.Base, quote.Price, rfq.Quote)
}
