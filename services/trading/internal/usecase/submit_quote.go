package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/vbloher/canton-private-otc-desk/pkg/decimal"
	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/trading/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/trading/internal/dto"
)

// SubmitQuote records the maker's quote as allocating and asks the ledger for the maker's
// committed allocations; the quote opens once they are on the ledger (Tick). When the ledger
// refuses, the quote fails and the error is returned.
func (u *UseCase) SubmitQuote(ctx context.Context, input dto.SubmitQuoteInput) (domain.Quote, error) {
	ctx, span := tracer.Start(ctx, "usecase SubmitQuote")
	defer span.End()

	price, err := decimal.Parse(input.Price)
	if err != nil {
		return domain.Quote{}, domain.Errorf(domain.ErrValidation, "price: %w", err)
	}

	var ttl time.Duration
	if input.TTLSeconds != nil {
		ttl = time.Duration(*input.TTLSeconds) * time.Second
	}

	desk, err := u.desk(ctx)
	if err != nil {
		return domain.Quote{}, fmt.Errorf("u.desk: %w", err)
	}

	d, err := u.saveQuote(ctx, input.RFQID, input.Maker, price, ttl, desk.Operator)
	if err != nil {
		return domain.Quote{}, fmt.Errorf("u.saveQuote: %w", err)
	}

	u.publishQuote(ctx, d.rfq.TakerParty, d.quote)

	// The quote is saved: its allocation goes ahead even when the caller goes away.
	ctx = context.WithoutCancel(ctx)

	err = u.ledger.Allocate(ctx, d.quote.ID, d.quote.SettlementDeadline(), d.legs, input.Maker.ID,
		describe(d.rfq, d.quote, false))
	if err != nil {
		u.failQuote(ctx, d.quote)

		return domain.Quote{}, fmt.Errorf("ledger.Allocate: %w", err)
	}

	return u.releaseIfDead(ctx, d.quote), nil
}

// releaseIfDead reads the quote again once its allocation is on the ledger. A quote that died
// meanwhile (its RFQ closed, say) was released before the allocation existed: release it again.
// It returns the quote as it is now.
func (u *UseCase) releaseIfDead(ctx context.Context, quote domain.Quote) domain.Quote {
	var current domain.Quote

	err := u.tx.Wrap(ctx, func(ctx context.Context) error {
		var err error

		_, current, err = u.lockQuote(ctx, quote.RFQID, quote.ID)
		if err != nil {
			return fmt.Errorf("u.lockQuote: %w", err)
		}

		return nil
	})
	if err != nil {
		log.Error().Err(err).Stringer("quote", quote.ID).Msg("usecase SubmitQuote: reading the quote again")

		return quote
	}

	if current.Status.Dead() {
		u.cancelSettlement(ctx, quote.ID)
	}

	return current
}

func (u *UseCase) saveQuote(
	ctx context.Context, rfqID uuid.UUID, maker domain.Party, price decimal.Decimal, ttl time.Duration,
	operator string,
) (deal, error) {
	var d deal

	err := u.tx.Wrap(ctx, func(ctx context.Context) error {
		var err error

		d.rfq, err = u.postgres.GetRFQForUpdate(ctx, rfqID)
		if err != nil {
			return fmt.Errorf("postgres.GetRFQForUpdate: %w", err)
		}

		d.quote, err = domain.NewQuote(d.rfq, maker, price, ttl, time.Now())
		if err != nil {
			return fmt.Errorf("domain.NewQuote: %w", err)
		}

		d.legs, err = domain.Legs(d.rfq, d.quote, operator)
		if err != nil {
			return fmt.Errorf("domain.Legs: %w", err)
		}

		err = u.postgres.CreateQuote(ctx, d.quote)
		if err != nil {
			return fmt.Errorf("postgres.CreateQuote: %w", err)
		}

		return nil
	})
	if err != nil {
		return deal{}, fmt.Errorf("transaction.Wrap: %w", err)
	}

	return d, nil
}

// failQuote fails a quote whose allocation the ledger refused, and releases whatever of it landed
// once the quote is dead: failed here, or closed meanwhile by a close that may have come before
// part of the allocation landed. An error from Allocate can be ambiguous: when a tick has seen the
// allocation land and opened the quote, or the taker accepted it, the quote keeps its allocation.
func (u *UseCase) failQuote(ctx context.Context, quote domain.Quote) {
	rfq, current, done, err := u.transition(ctx, quote, func(q domain.Quote) (domain.Quote, bool) {
		return q.WithStatus(domain.QuoteFailed), q.Status == domain.QuoteAllocating
	})
	if err != nil {
		log.Error().Err(err).Stringer("quote", quote.ID).Msg("usecase SubmitQuote: u.transition")

		return
	}

	if current.Status.Dead() {
		u.cancelSettlement(ctx, quote.ID)
	}

	if done {
		u.publishQuote(ctx, rfq.TakerParty, current)
	}
}
