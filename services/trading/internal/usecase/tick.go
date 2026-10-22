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
)

const settleTimedOut = "settlement timed out"

// Tick advances the book; the worker runs it every WORKER_INTERVAL:
//  1. live quotes past their validUntil expire and their allocations are released;
//  2. open RFQs past their deadline without a settling trade expire, with their live quotes;
//  3. one ledger read covers every allocating quote and settling trade: a quote whose maker has
//     allocated opens, a trade whose parties have all allocated settles (its quote wins, the RFQ
//     fills, the other live quotes lose), and a trade settling longer than SETTLE_TIMEOUT fails.
func (u *UseCase) Tick(ctx context.Context, now time.Time) error {
	ctx, span := tracer.Start(ctx, "usecase Tick")
	defer span.End()

	quotesErr := u.expireQuotes(ctx, now)
	rfqsErr := u.expireRFQs(ctx, now)
	settlementsErr := u.advanceSettlements(ctx, now)

	return errors.Join(quotesErr, rfqsErr, settlementsErr)
}

func (u *UseCase) expireQuotes(ctx context.Context, now time.Time) error {
	live, err := u.postgres.ListQuotesByStatus(ctx, domain.QuoteAllocating, domain.QuoteOpen)
	if err != nil {
		return fmt.Errorf("postgres.ListQuotesByStatus: %w", err)
	}

	var errs []error

	for _, q := range live {
		if q.ValidUntil.After(now) {
			continue
		}

		rfq, expired, done, err := u.transition(ctx, q, func(current domain.Quote) (domain.Quote, bool) {
			return current.WithStatus(domain.QuoteExpired), current.Status.Live() && !current.ValidUntil.After(now)
		})
		if err != nil {
			errs = append(errs, fmt.Errorf("expire quote %s: %w", q.ID, err))

			continue
		}

		if done {
			u.release(ctx, rfq.TakerParty, expired)
		}
	}

	return errors.Join(errs...)
}

func (u *UseCase) expireRFQs(ctx context.Context, now time.Time) error {
	rfqs, err := u.postgres.ListExpiredRFQs(ctx, now)
	if err != nil {
		return fmt.Errorf("postgres.ListExpiredRFQs: %w", err)
	}

	var errs []error

	for _, rfq := range rfqs {
		err = u.expireRFQ(ctx, rfq.ID, now)
		if err != nil {
			errs = append(errs, fmt.Errorf("expire rfq %s: %w", rfq.ID, err))
		}
	}

	return errors.Join(errs...)
}

func (u *UseCase) expireRFQ(ctx context.Context, id uuid.UUID, now time.Time) error {
	var (
		rfq     domain.RFQ
		expired []domain.Quote
		done    bool
	)

	err := u.tx.Wrap(ctx, func(ctx context.Context) error {
		var err error

		rfq, err = u.postgres.GetRFQForUpdate(ctx, id)
		if err != nil {
			return fmt.Errorf("postgres.GetRFQForUpdate: %w", err)
		}

		if rfq.Status != domain.RFQOpen || rfq.Deadline.After(now) {
			return nil
		}

		quotes, err := u.postgres.ListQuotes(ctx, []uuid.UUID{id})
		if err != nil {
			return fmt.Errorf("postgres.ListQuotes: %w", err)
		}

		// ListExpiredRFQs leaves out RFQs with a settling trade, but an accept may have
		// committed since: such an RFQ waits for its trade.
		if settling(quotes) {
			return nil
		}

		rfq, expired, err = u.closeRFQ(ctx, rfq, quotes, domain.RFQExpired, domain.QuoteExpired, now)
		if err != nil {
			return fmt.Errorf("u.closeRFQ: %w", err)
		}

		done = true

		return nil
	})
	if err != nil {
		return fmt.Errorf("transaction.Wrap: %w", err)
	}

	if done {
		u.closed(ctx, rfq, expired)
	}

	return nil
}

func (u *UseCase) advanceSettlements(ctx context.Context, now time.Time) error {
	quotes, err := u.postgres.ListQuotesByStatus(ctx, domain.QuoteAllocating)
	if err != nil {
		return fmt.Errorf("postgres.ListQuotesByStatus: %w", err)
	}

	trades, err := u.postgres.ListSettlingTrades(ctx)
	if err != nil {
		return fmt.Errorf("postgres.ListSettlingTrades: %w", err)
	}

	if len(quotes) == 0 && len(trades) == 0 {
		return nil
	}

	desk, err := u.desk(ctx)
	if err != nil {
		return fmt.Errorf("u.desk: %w", err)
	}

	queries, err := u.settlementQueries(ctx, quotes, trades, desk.Operator)
	if err != nil {
		return fmt.Errorf("u.settlementQueries: %w", err)
	}

	allocated, err := u.ledger.Settlements(ctx, queries)
	if err != nil {
		return fmt.Errorf("ledger.Settlements: %w", err)
	}

	errs := make([]error, 0, len(quotes)+len(trades))

	for _, q := range quotes {
		errs = append(errs, u.advanceQuote(ctx, q, queries, allocated, now))
	}

	for _, t := range trades {
		errs = append(errs, u.advanceTrade(ctx, t, queries, allocated, now))
	}

	return errors.Join(errs...)
}

func (u *UseCase) advanceQuote(
	ctx context.Context, q domain.Quote, queries map[uuid.UUID][]domain.Leg,
	allocated map[uuid.UUID]map[string]bool, now time.Time,
) error {
	if _, ok := queries[q.ID]; !ok || !allocated[q.ID][q.Maker.ID] {
		return nil
	}

	return u.openQuote(ctx, q, now)
}

func (u *UseCase) advanceTrade(
	ctx context.Context, t domain.Trade, queries map[uuid.UUID][]domain.Leg,
	allocated map[uuid.UUID]map[string]bool, now time.Time,
) error {
	timedOut := now.Sub(t.CreatedAt) > u.config.SettleTimeout
	legs, ok := queries[t.QuoteID]

	switch {
	case ok && domain.Allocated(legs, allocated[t.QuoteID]):
		err := u.settle(ctx, t, legs, now)
		if err != nil && timedOut {
			// A settlement the ledger keeps refusing must not lock the parties' funds forever.
			return errors.Join(fmt.Errorf("u.settle: %w", err), u.failTrade(ctx, t, settleTimedOut))
		}

		return wrapSettle(err)
	case ok && domain.NoneAllocated(legs, allocated[t.QuoteID]):
		// Every allocation is gone. While a trade settles only a settle consumes them (the desk
		// cancels them once the trade has failed): an earlier settle landed and its answer or its
		// record was lost. Settling again finds them gone and records the trade. A ledger failure
		// waits for the next tick, never for the timeout, which would free the RFQ for a second fill.
		return wrapSettle(u.settle(ctx, t, legs, now))
	case timedOut:
		return u.failTrade(ctx, t, settleTimedOut)
	default:
		return nil
	}
}

func wrapSettle(err error) error {
	if err != nil {
		return fmt.Errorf("u.settle: %w", err)
	}

	return nil
}

// settlementQueries are the legs of every allocating quote and settling trade, by settlement id.
func (u *UseCase) settlementQueries(
	ctx context.Context, quotes []domain.Quote, trades []domain.Trade, operator string,
) (map[uuid.UUID][]domain.Leg, error) {
	rfqs, err := u.rfqsOf(ctx, quotes)
	if err != nil {
		return nil, fmt.Errorf("u.rfqsOf: %w", err)
	}

	queries := make(map[uuid.UUID][]domain.Leg, len(quotes)+len(trades))

	for _, q := range quotes {
		rfq, ok := rfqs[q.RFQID]
		if !ok {
			log.Error().Stringer("quote", q.ID).Msg("usecase Tick: the quote's rfq is missing")

			continue
		}

		legs, err := domain.Legs(rfq, q, operator)
		if err != nil {
			log.Error().Err(err).Stringer("quote", q.ID).Msg("usecase Tick: domain.Legs")

			continue
		}

		queries[q.ID] = legs
	}

	for _, t := range trades {
		legs, err := t.Legs(operator)
		if err != nil {
			log.Error().Err(err).Stringer("trade", t.ID).Msg("usecase Tick: trade.Legs")

			continue
		}

		queries[t.QuoteID] = legs
	}

	return queries, nil
}

// rfqsOf reads the RFQs of the quotes, by id.
func (u *UseCase) rfqsOf(ctx context.Context, quotes []domain.Quote) (map[uuid.UUID]domain.RFQ, error) {
	byID := make(map[uuid.UUID]domain.RFQ, len(quotes))
	if len(quotes) == 0 {
		return byID, nil
	}

	ids := make([]uuid.UUID, 0, len(quotes))
	seen := make(map[uuid.UUID]bool, len(quotes))

	for _, q := range quotes {
		if !seen[q.RFQID] {
			seen[q.RFQID] = true
			ids = append(ids, q.RFQID)
		}
	}

	rfqs, err := u.postgres.ListRFQs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("postgres.ListRFQs: %w", err)
	}

	for _, rfq := range rfqs {
		byID[rfq.ID] = rfq
	}

	return byID, nil
}

func (u *UseCase) openQuote(ctx context.Context, q domain.Quote, now time.Time) error {
	rfq, opened, done, err := u.transition(ctx, q, func(current domain.Quote) (domain.Quote, bool) {
		return current.Open(now), current.Status == domain.QuoteAllocating && current.ValidUntil.After(now)
	})
	if err != nil {
		return fmt.Errorf("open quote %s: %w", q.ID, err)
	}

	if done {
		u.publishQuote(ctx, rfq.TakerParty, opened)
	}

	return nil
}

type settlement struct {
	rfq   domain.RFQ
	trade domain.Trade
	won   domain.Quote
	lost  []domain.Quote
	done  bool
}

// settle settles the trade on the ledger and records it. A stopping worker starts no settlement,
// but nothing cuts short one already started: once the ledger is asked to settle, the record must
// follow, and so must the releases the record commits. A ledger failure leaves the trade settling:
// the next tick tries again, until SETTLE_TIMEOUT fails it.
func (u *UseCase) settle(ctx context.Context, trade domain.Trade, legs []domain.Leg, now time.Time) error {
	err := ctx.Err()
	if err != nil {
		return fmt.Errorf("settlement of trade %s not started: %w", trade.ID, err)
	}

	ctx = context.WithoutCancel(ctx)

	updateID, err := u.ledger.Settle(ctx, trade.QuoteID, legs)

	switch {
	case errors.Is(err, domain.ErrSettlementGone):
		// Only the desk's own settle consumes a settling trade's allocations (the desk cancels them
		// only once the trade has failed): an earlier settle landed and its answer was lost.
		log.Warn().Err(err).Stringer("trade", trade.ID).
			Msg("usecase Tick: the settlement's allocations are consumed already, recorded as settled")
	case err != nil:
		return fmt.Errorf("ledger.Settle %s: %w", trade.ID, err)
	}

	s, err := u.recordSettlement(ctx, trade.Settled(updateID, now), now)
	if err != nil {
		return fmt.Errorf("record settlement %s: %w", trade.ID, err)
	}

	if s.done {
		u.publishTrade(ctx, s.trade)
		u.publishQuote(ctx, s.rfq.TakerParty, s.won)
		u.closed(ctx, s.rfq, s.lost)
	}

	return nil
}

// recordSettlement saves a settled trade: its quote won, its RFQ filled, every other live quote lost.
func (u *UseCase) recordSettlement(ctx context.Context, trade domain.Trade, now time.Time) (settlement, error) {
	s := settlement{trade: trade}

	err := u.tx.Wrap(ctx, func(ctx context.Context) error {
		rfq, quote, err := u.lockQuote(ctx, trade.RFQID, trade.QuoteID)
		if err != nil {
			return fmt.Errorf("u.lockQuote: %w", err)
		}

		if quote.Status != domain.QuoteAccepted {
			unrecorded(trade, quote.Status)

			return nil
		}

		quotes, err := u.postgres.ListQuotes(ctx, []uuid.UUID{rfq.ID})
		if err != nil {
			return fmt.Errorf("postgres.ListQuotes: %w", err)
		}

		err = u.postgres.UpdateTrade(ctx, trade)
		if err != nil {
			return fmt.Errorf("postgres.UpdateTrade: %w", err)
		}

		s.won = quote.WithStatus(domain.QuoteWon)

		err = u.postgres.UpdateQuote(ctx, s.won)
		if err != nil {
			return fmt.Errorf("postgres.UpdateQuote: %w", err)
		}

		s.rfq, s.lost, err = u.closeRFQ(ctx, rfq, quotes, domain.RFQFilled, domain.QuoteLost, now)
		if err != nil {
			return fmt.Errorf("u.closeRFQ: %w", err)
		}

		s.done = true

		return nil
	})
	if err != nil {
		return settlement{}, fmt.Errorf("transaction.Wrap: %w", err)
	}

	return s, nil
}

// unrecorded reports the ledger's answer to a settle (an update id, or allocations already gone)
// for a trade that is no longer settling here.
func unrecorded(trade domain.Trade, status domain.QuoteStatus) {
	if status == domain.QuoteWon {
		log.Warn().Stringer("trade", trade.ID).Str("update", trade.UpdateID).
			Msg("usecase Tick: the settlement is recorded already")

		return
	}

	log.Error().Stringer("trade", trade.ID).Str("update", trade.UpdateID).Str("quote", string(status)).
		Msg("usecase Tick: the ledger answered the settle, but the trade is no longer settling: not recorded")
}
