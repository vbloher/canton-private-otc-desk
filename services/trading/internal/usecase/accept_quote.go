package usecase

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/trading/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/trading/internal/dto"
)

func (u *UseCase) AcceptQuote(ctx context.Context, input dto.AcceptQuoteInput) (domain.Trade, error) {
	ctx, span := tracer.Start(ctx, "usecase AcceptQuote")
	defer span.End()

	desk, err := u.desk(ctx)
	if err != nil {
		return domain.Trade{}, fmt.Errorf("u.desk: %w", err)
	}

	a, err := u.accept(ctx, input, desk.Operator)
	if err != nil {
		return domain.Trade{}, fmt.Errorf("u.accept: %w", err)
	}

	u.publishQuote(ctx, a.rfq.TakerParty, a.quote)
	u.publishTrade(ctx, a.trade)

	ctx = context.WithoutCancel(ctx)

	err = u.allocateTrade(ctx, a, desk.Operator)
	if err != nil {
		failErr := u.failTrade(ctx, a.trade, failureReason(err))
		if failErr != nil {
			log.Error().Err(failErr).Stringer("trade", a.trade.ID).Msg("usecase AcceptQuote: u.failTrade")
		}

		return domain.Trade{}, fmt.Errorf("u.allocateTrade: %w", err)
	}

	return a.trade, nil
}

func (u *UseCase) accept(ctx context.Context, input dto.AcceptQuoteInput, operator string) (deal, error) {
	var a deal

	err := u.tx.Wrap(ctx, func(ctx context.Context) error {
		found, err := u.postgres.GetQuote(ctx, input.QuoteID)
		if err != nil {
			return fmt.Errorf("postgres.GetQuote: %w", err)
		}

		a.rfq, a.quote, err = u.lockQuote(ctx, found.RFQID, found.ID)
		if err != nil {
			return fmt.Errorf("u.lockQuote: %w", err)
		}

		now := time.Now()

		err = u.checkAccept(ctx, a.rfq, a.quote, input.Taker, now)
		if err != nil {
			return fmt.Errorf("u.checkAccept: %w", err)
		}

		a.legs, err = domain.Legs(a.rfq, a.quote, operator)
		if err != nil {
			return fmt.Errorf("domain.Legs: %w", err)
		}

		a.quote = a.quote.WithStatus(domain.QuoteAccepted)
		a.trade = domain.NewTrade(a.rfq, a.quote, now)

		err = u.postgres.UpdateQuote(ctx, a.quote)
		if err != nil {
			return fmt.Errorf("postgres.UpdateQuote: %w", err)
		}

		err = u.postgres.CreateTrade(ctx, a.trade)
		if err != nil {
			return fmt.Errorf("postgres.CreateTrade: %w", err)
		}

		return nil
	})
	if err != nil {
		return deal{}, fmt.Errorf("transaction.Wrap: %w", err)
	}

	return a, nil
}

func (u *UseCase) checkAccept(
	ctx context.Context, rfq domain.RFQ, quote domain.Quote, taker domain.Party, now time.Time,
) error {
	if rfq.TakerParty != taker.ID {
		return domain.ErrForbidden
	}

	if !quote.Acceptable(now) {
		return domain.ErrQuoteNotOpen
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

	return nil
}

func (u *UseCase) allocateTrade(ctx context.Context, a deal, operator string) error {
	deadline := a.quote.SettlementDeadline()

	if domain.HasFee(a.legs) {
		err := u.ledger.Allocate(ctx, a.quote.ID, deadline, a.legs, operator,
			fmt.Sprintf("Desk fee %s %s", a.trade.Fee, a.trade.Quote))
		if err != nil {
			return fmt.Errorf("ledger.Allocate operator: %w", err)
		}
	}

	err := u.ledger.Allocate(ctx, a.quote.ID, deadline, a.legs, a.rfq.TakerParty, describe(a.rfq, a.quote, true))
	if err != nil {
		return fmt.Errorf("ledger.Allocate taker: %w", err)
	}

	return nil
}

func failureReason(err error) string {
	if errors.Is(err, domain.ErrInsufficientFunds) {
		return "insufficient funds"
	}

	return "allocation failed"
}
