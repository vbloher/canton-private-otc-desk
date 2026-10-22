package usecase_test

import (
	"bytes"
	"context"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/services/trading/internal/domain"
)

// book is what the four reads of a tick return.
type book struct {
	live       []domain.Quote // allocating and open quotes, for the expiry step
	expired    []domain.RFQ   // ListExpiredRFQs
	allocating []domain.Quote // for the settlement step
	settling   []domain.Trade
}

func (d deps) expectTick(now time.Time, b book) {
	d.postgres.EXPECT().ListQuotesByStatus(Any, []domain.QuoteStatus{domain.QuoteAllocating, domain.QuoteOpen}).
		Return(b.live, nil).Once()
	d.postgres.EXPECT().ListExpiredRFQs(Any, at(now)).Return(b.expired, nil)
	d.postgres.EXPECT().ListQuotesByStatus(Any, []domain.QuoteStatus{domain.QuoteAllocating}).
		Return(b.allocating, nil).Once()
	d.postgres.EXPECT().ListSettlingTrades(Any).Return(b.settling, nil)
}

func Test_Tick_IdleDeskDoesNotCallTheLedger(t *testing.T) {
	d := newDeps(t)
	now := time.Now()

	d.expectTick(now, book{})

	require.NoError(t, d.useCase(t).Tick(t.Context(), now))
}

func Test_Tick_ExpiresQuote(t *testing.T) {
	d := newDeps(t)
	now := time.Now()
	rfq := openRFQ(now.Add(-2 * time.Minute))
	stale := quoteOf(rfq, makerA, "100000", domain.QuoteOpen, now)
	fresh := quoteOf(rfq, makerB, "100100", domain.QuoteOpen, now.Add(time.Second))

	d.expectTick(now, book{live: []domain.Quote{stale, fresh}})
	d.postgres.EXPECT().GetRFQForUpdate(Any, rfq.ID).Return(rfq, nil)
	d.postgres.EXPECT().GetQuoteForUpdate(Any, stale.ID).Return(stale, nil)
	d.postgres.EXPECT().UpdateQuote(Any, quoteIn(stale.ID, domain.QuoteExpired)).Return(nil)
	d.ledger.EXPECT().CancelSettlement(Any, stale.ID).Return(nil)
	d.events.EXPECT().Publish(Any, quoteEvent(stale, domain.QuoteExpired), parties(taker, makerA)).Return(nil)

	require.NoError(t, d.useCase(t).Tick(t.Context(), now))
}

func Test_Tick_OpensQuoteWhenMakerAllocated(t *testing.T) {
	d := newDeps(t)
	now := time.Now()
	rfq := openRFQ(now.Add(-time.Minute))
	ready := quoteOf(rfq, makerA, "100000", domain.QuoteAllocating, now.Add(time.Minute))
	waiting := quoteOf(rfq, makerB, "100100", domain.QuoteAllocating, now.Add(time.Minute))
	readyLegs := legsOf(t, rfq, ready)

	d.expectTick(now, book{live: []domain.Quote{ready, waiting}, allocating: []domain.Quote{ready, waiting}})
	d.cachedDesk()
	d.postgres.EXPECT().ListRFQs(Any, []uuid.UUID{rfq.ID}).Return([]domain.RFQ{rfq}, nil)
	d.ledger.EXPECT().Settlements(Any, mock.MatchedBy(func(q map[uuid.UUID][]domain.Leg) bool {
		return len(q) == 2 && slices.Equal(show(readyLegs), show(q[ready.ID]))
	})).Return(map[uuid.UUID]map[string]bool{
		ready.ID:   {makerA.ID: true, taker.ID: false, operator: false},
		waiting.ID: {makerB.ID: false},
	}, nil)
	d.postgres.EXPECT().GetRFQForUpdate(Any, rfq.ID).Return(rfq, nil)
	d.postgres.EXPECT().GetQuoteForUpdate(Any, ready.ID).Return(ready, nil)
	d.postgres.EXPECT().UpdateQuote(Any, mock.MatchedBy(func(q domain.Quote) bool {
		return q.ID == ready.ID && q.Status == domain.QuoteOpen && q.OpenedAt != nil && q.OpenedAt.Equal(now.Truncate(time.Microsecond))
	})).Return(nil)
	d.events.EXPECT().Publish(Any, quoteEvent(ready, domain.QuoteOpen), parties(taker, makerA)).Return(nil)

	require.NoError(t, d.useCase(t).Tick(t.Context(), now))
}

func Test_Tick_SettlesWhenAllAllocated(t *testing.T) {
	d := newDeps(t)
	now := time.Now()
	rfq := openRFQ(now.Add(-2 * time.Minute))
	won := quoteOf(rfq, makerA, "100000", domain.QuoteAccepted, now.Add(time.Minute))
	openSibling := quoteOf(rfq, makerB, "100100", domain.QuoteOpen, now.Add(time.Minute))
	allocatingSibling := quoteOf(rfq, makerC, "100200", domain.QuoteAllocating, now.Add(time.Minute))
	trade := domain.NewTrade(rfq, won, now.Add(-time.Minute))

	legs, err := trade.Legs(operator)
	require.NoError(t, err)

	d.expectTick(now, book{
		live:       []domain.Quote{openSibling, allocatingSibling},
		allocating: []domain.Quote{allocatingSibling},
		settling:   []domain.Trade{trade},
	})
	d.cachedDesk()
	d.postgres.EXPECT().ListRFQs(Any, []uuid.UUID{rfq.ID}).Return([]domain.RFQ{rfq}, nil)
	d.ledger.EXPECT().Settlements(Any, mock.MatchedBy(func(q map[uuid.UUID][]domain.Leg) bool {
		return len(q) == 2 && slices.Equal(show(legs), show(q[won.ID]))
	})).Return(map[uuid.UUID]map[string]bool{
		won.ID:               {taker.ID: true, makerA.ID: true, operator: true},
		allocatingSibling.ID: {makerC.ID: false},
	}, nil)
	d.ledger.EXPECT().Settle(Any, won.ID, sameLegs(legs)).Return("update-1", nil)
	d.postgres.EXPECT().GetRFQForUpdate(Any, rfq.ID).Return(rfq, nil)
	d.postgres.EXPECT().GetQuoteForUpdate(Any, won.ID).Return(won, nil)
	d.postgres.EXPECT().ListQuotes(Any, []uuid.UUID{rfq.ID}).
		Return([]domain.Quote{won, openSibling, allocatingSibling}, nil)

	var settled domain.Trade

	d.postgres.EXPECT().UpdateTrade(Any, Any).Run(func(_ context.Context, tr domain.Trade) { settled = tr }).Return(nil)
	d.postgres.EXPECT().UpdateQuote(Any, quoteIn(won.ID, domain.QuoteWon)).Return(nil)

	var filled domain.RFQ

	d.postgres.EXPECT().UpdateRFQ(Any, Any).Run(func(_ context.Context, r domain.RFQ) { filled = r }).Return(nil)

	for _, q := range []domain.Quote{openSibling, allocatingSibling} {
		d.postgres.EXPECT().UpdateQuote(Any, quoteIn(q.ID, domain.QuoteLost)).Return(nil)
		d.ledger.EXPECT().CancelSettlement(Any, q.ID).Return(nil).Once()
	}

	// The taker hears of the quote it saw; the one that never opened stays between its maker and the desk.
	d.events.EXPECT().Publish(Any, quoteEvent(openSibling, domain.QuoteLost), parties(taker, makerB)).Return(nil)
	d.events.EXPECT().Publish(Any, quoteEvent(allocatingSibling, domain.QuoteLost), parties(makerC)).Return(nil)

	d.events.EXPECT().Publish(Any, tradeEvent(trade, domain.TradeSettled), parties(taker, makerA)).Return(nil)
	d.events.EXPECT().Publish(Any, quoteEvent(won, domain.QuoteWon), parties(taker, makerA)).Return(nil)
	d.events.EXPECT().Publish(Any, rfqUpdated(rfq), parties(taker, makerA, makerB, makerC)).Return(nil)

	require.NoError(t, d.useCase(t).Tick(t.Context(), now))
	require.Equal(t, trade.ID, settled.ID)
	require.Equal(t, domain.TradeSettled, settled.Status)
	require.Equal(t, "update-1", settled.UpdateID)
	require.NotNil(t, settled.SettledAt)
	require.Equal(t, domain.RFQFilled, filled.Status)
	require.NotNil(t, filled.ClosedAt)
}

func Test_Tick_ExpiresRFQ(t *testing.T) {
	d := newDeps(t)
	now := time.Now()
	rfq := openRFQ(now.Add(-6 * time.Minute)) // its deadline passed a minute ago
	// The quote step expires quotes past validUntil first; this one it did not get to.
	late := quoteOf(rfq, makerA, "100000", domain.QuoteAllocating, rfq.Deadline)
	expired := quoteOf(rfq, makerB, "100100", domain.QuoteExpired, rfq.Deadline)

	d.expectTick(now, book{expired: []domain.RFQ{rfq}})
	d.postgres.EXPECT().GetRFQForUpdate(Any, rfq.ID).Return(rfq, nil)
	d.postgres.EXPECT().ListQuotes(Any, []uuid.UUID{rfq.ID}).Return([]domain.Quote{late, expired}, nil)

	var closed domain.RFQ

	d.postgres.EXPECT().UpdateRFQ(Any, Any).Run(func(_ context.Context, r domain.RFQ) { closed = r }).Return(nil)
	d.postgres.EXPECT().UpdateQuote(Any, quoteIn(late.ID, domain.QuoteExpired)).Return(nil)
	d.ledger.EXPECT().CancelSettlement(Any, late.ID).Return(nil)
	d.events.EXPECT().Publish(Any, quoteEvent(late, domain.QuoteExpired), parties(makerA)).Return(nil) // never opened
	d.events.EXPECT().Publish(Any, rfqUpdated(rfq), parties(taker, makerA, makerB, makerC)).Return(nil)

	require.NoError(t, d.useCase(t).Tick(t.Context(), now))
	require.Equal(t, domain.RFQExpired, closed.Status)
	require.NotNil(t, closed.ClosedAt)
}

func Test_Tick_KeepsRFQWithSettlingTrade(t *testing.T) {
	d := newDeps(t)
	now := time.Now()
	rfq := openRFQ(now.Add(-6 * time.Minute)) // its deadline passed a minute ago
	accepted := quoteOf(rfq, makerA, "100000", domain.QuoteAccepted, rfq.Deadline)
	trade := domain.NewTrade(rfq, accepted, now.Add(-2*time.Minute))

	// ListExpiredRFQs leaves out an RFQ with a settling trade, but an accept may commit after
	// that read: the use case checks again under the RFQ row lock.
	d.expectTick(now, book{expired: []domain.RFQ{rfq}, settling: []domain.Trade{trade}})
	d.postgres.EXPECT().GetRFQForUpdate(Any, rfq.ID).Return(rfq, nil)
	d.postgres.EXPECT().ListQuotes(Any, []uuid.UUID{rfq.ID}).Return([]domain.Quote{accepted}, nil)
	d.cachedDesk()
	d.ledger.EXPECT().Settlements(Any, Any).Return(map[uuid.UUID]map[string]bool{
		accepted.ID: {taker.ID: false, makerA.ID: true, operator: true},
	}, nil)

	// No UpdateRFQ, no UpdateQuote, no event: the strict mocks fail on any of them.
	require.NoError(t, d.useCase(t).Tick(t.Context(), now))
}

func Test_Tick_FailsTradeWhoseSettleKeepsFailing(t *testing.T) {
	d := newDeps(t)
	now := time.Now()
	rfq := openRFQ(now.Add(-12 * time.Minute))
	rfq.Deadline = rfq.CreatedAt.Add(time.Hour)
	accepted := quoteOf(rfq, makerA, "100000", domain.QuoteAccepted, now.Add(-8*time.Minute))
	trade := domain.NewTrade(rfq, accepted, now.Add(-11*time.Minute))
	errRejected := domain.Errorf(domain.ErrUpstream, "settlement rejected")

	d.expectTick(now, book{settling: []domain.Trade{trade}})
	d.cachedDesk()
	d.ledger.EXPECT().Settlements(Any, Any).Return(map[uuid.UUID]map[string]bool{
		accepted.ID: {taker.ID: true, makerA.ID: true, operator: true},
	}, nil)
	d.ledger.EXPECT().Settle(Any, accepted.ID, Any).Return("", errRejected)
	d.postgres.EXPECT().GetRFQForUpdate(Any, rfq.ID).Return(rfq, nil)
	d.postgres.EXPECT().GetQuoteForUpdate(Any, accepted.ID).Return(accepted, nil)
	d.postgres.EXPECT().UpdateTrade(Any, mock.MatchedBy(func(tr domain.Trade) bool {
		return tr.ID == trade.ID && tr.Status == domain.TradeFailed && tr.Reason == "settlement timed out"
	})).Return(nil)
	d.postgres.EXPECT().UpdateQuote(Any, quoteIn(accepted.ID, domain.QuoteFailed)).Return(nil)
	d.ledger.EXPECT().CancelSettlement(Any, accepted.ID).Return(nil)
	d.events.EXPECT().Publish(Any, tradeEvent(trade, domain.TradeFailed), parties(taker, makerA)).Return(nil)
	d.events.EXPECT().Publish(Any, quoteEvent(accepted, domain.QuoteFailed), parties(makerA)).Return(nil)

	err := d.useCase(t).Tick(t.Context(), now)
	require.ErrorIs(t, err, errRejected, "the settle failure is still reported")
}

func Test_Tick_FailsTradeAfterTimeout(t *testing.T) {
	d := newDeps(t)
	now := time.Now()
	rfq := openRFQ(now.Add(-12 * time.Minute))
	rfq.Deadline = rfq.CreatedAt.Add(time.Hour)
	accepted := quoteOf(rfq, makerA, "100000", domain.QuoteAccepted, now.Add(-8*time.Minute))
	trade := domain.NewTrade(rfq, accepted, now.Add(-11*time.Minute))

	d.expectTick(now, book{settling: []domain.Trade{trade}})
	d.cachedDesk()
	d.ledger.EXPECT().Settlements(Any, Any).Return(map[uuid.UUID]map[string]bool{
		accepted.ID: {taker.ID: false, makerA.ID: true, operator: true},
	}, nil)
	d.postgres.EXPECT().GetRFQForUpdate(Any, rfq.ID).Return(rfq, nil)
	d.postgres.EXPECT().GetQuoteForUpdate(Any, accepted.ID).Return(accepted, nil)

	var failed domain.Trade

	d.postgres.EXPECT().UpdateTrade(Any, Any).Run(func(_ context.Context, tr domain.Trade) { failed = tr }).Return(nil)
	d.postgres.EXPECT().UpdateQuote(Any, quoteIn(accepted.ID, domain.QuoteFailed)).Return(nil)
	d.ledger.EXPECT().CancelSettlement(Any, accepted.ID).Return(nil)
	d.events.EXPECT().Publish(Any, tradeEvent(trade, domain.TradeFailed), parties(taker, makerA)).Return(nil)
	d.events.EXPECT().Publish(Any, quoteEvent(accepted, domain.QuoteFailed), parties(makerA)).Return(nil)

	require.NoError(t, d.useCase(t).Tick(t.Context(), now))
	require.Equal(t, trade.ID, failed.ID)
	require.Equal(t, domain.TradeFailed, failed.Status)
	require.Equal(t, "settlement timed out", failed.Reason)
}

// settledFixture is a settling trade on makerA's accepted quote, with makerB's open quote beside it.
type settledFixture struct {
	rfq     domain.RFQ
	won     domain.Quote
	sibling domain.Quote
	trade   domain.Trade
}

func newSettledFixture(now time.Time) settledFixture {
	rfq := openRFQ(now.Add(-2 * time.Minute))
	won := quoteOf(rfq, makerA, "100000", domain.QuoteAccepted, now.Add(time.Minute))

	return settledFixture{
		rfq:     rfq,
		won:     won,
		sibling: quoteOf(rfq, makerB, "100100", domain.QuoteOpen, now.Add(time.Minute)),
		trade:   domain.NewTrade(rfq, won, now.Add(-time.Minute)),
	}
}

// expectAnswer runs a tick over the fixture's trade, the ledger answering answer for its settlement.
func (d deps) expectAnswer(now time.Time, f settledFixture, answer map[string]bool) {
	d.expectTick(now, book{live: []domain.Quote{f.sibling}, settling: []domain.Trade{f.trade}})
	d.cachedDesk()
	d.ledger.EXPECT().Settlements(Any, Any).Return(map[uuid.UUID]map[string]bool{f.won.ID: answer}, nil)
}

// expectTradeLocked locks the trade's RFQ and the won quote.
func (d deps) expectTradeLocked(f settledFixture) {
	d.postgres.EXPECT().GetRFQForUpdate(Any, f.rfq.ID).Return(f.rfq, nil)
	d.postgres.EXPECT().GetQuoteForUpdate(Any, f.won.ID).Return(f.won, nil)
}

// expectAllocated reads the trade's settlement as complete and locks the RFQ and the won quote.
func (d deps) expectAllocated(now time.Time, f settledFixture) {
	d.expectAnswer(now, f, map[string]bool{taker.ID: true, makerA.ID: true, operator: true})
	d.expectTradeLocked(f)
}

// expectRecorded saves the settlement into *settled and requires that every ledger release and
// event after the commit still runs on a live context.
func (d deps) expectRecorded(t *testing.T, f settledFixture, settled *domain.Trade) {
	t.Helper()

	live := mock.MatchedBy(func(ctx context.Context) bool { return ctx.Err() == nil })

	d.postgres.EXPECT().ListQuotes(Any, []uuid.UUID{f.rfq.ID}).Return([]domain.Quote{f.won, f.sibling}, nil)
	d.postgres.EXPECT().UpdateTrade(Any, Any).Run(func(_ context.Context, tr domain.Trade) { *settled = tr }).Return(nil)
	d.postgres.EXPECT().UpdateQuote(Any, quoteIn(f.won.ID, domain.QuoteWon)).Return(nil)
	d.postgres.EXPECT().UpdateRFQ(Any, Any).Return(nil)
	d.postgres.EXPECT().UpdateQuote(Any, quoteIn(f.sibling.ID, domain.QuoteLost)).Return(nil)
	d.ledger.EXPECT().CancelSettlement(live, f.sibling.ID).Return(nil)
	d.events.EXPECT().Publish(live, Any, Any).Return(nil).Times(4) // trade, won, lost, rfq
}

// SIGTERM while the ledger settles: the swap happened, so the trade must still be recorded.
func Test_Tick_RecordsTheSettlementWhenStoppedAfterSettle(t *testing.T) {
	d := newDeps(t)
	now := time.Now()
	f := newSettledFixture(now)
	ctx, stop := context.WithCancel(t.Context())

	var settled domain.Trade

	d.expectAllocated(now, f)
	d.ledger.EXPECT().Settle(Any, f.won.ID, Any).RunAndReturn(
		func(ctx context.Context, _ uuid.UUID, _ []domain.Leg) (string, error) {
			stop()
			require.NoError(t, ctx.Err(), "Settle runs on a context the stop does not reach")

			return "update-1", nil
		})
	d.expectRecorded(t, f, &settled)

	require.NoError(t, d.useCase(t).Tick(ctx, now))
	require.Equal(t, f.trade.ID, settled.ID)
	require.Equal(t, domain.TradeSettled, settled.Status)
	require.Equal(t, "update-1", settled.UpdateID)
}

// An earlier settle landed, but its answer or its record was lost. Its allocations are archived, so
// the ledger reports every party unallocated, and only a settle consumes a settling trade's
// allocations (the desk cancels them once the trade has failed). Settling again finds them gone and
// records the trade settled, past SETTLE_TIMEOUT too: failing it would free the RFQ for a second fill.
func Test_Tick_RecordsASettlementWhoseAllocationsAreGone(t *testing.T) {
	gone := map[string]bool{taker.ID: false, makerA.ID: false, operator: false}

	cases := []struct {
		name   string
		feeBps int64
		age    time.Duration
		answer map[string]bool
	}{
		{"with a fee", 10, time.Minute, gone},
		{"without a fee", 0, time.Minute, map[string]bool{taker.ID: false, makerA.ID: false}},
		{"past SETTLE_TIMEOUT", 10, 11 * time.Minute, gone},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := newDeps(t)
			now := time.Now()
			f := newSettledFixture(now)
			f.rfq.FeeBps = c.feeBps
			f.rfq.CreatedAt = now.Add(-c.age - time.Minute)
			f.rfq.Deadline = f.rfq.CreatedAt.Add(time.Hour)
			f.trade = domain.NewTrade(f.rfq, f.won, now.Add(-c.age))

			legs, err := f.trade.Legs(operator)
			require.NoError(t, err)

			var settled domain.Trade

			d.expectAnswer(now, f, c.answer)
			d.ledger.EXPECT().Settle(Any, f.won.ID, sameLegs(legs)).
				Return("", domain.Errorf(domain.ErrSettlementGone, "no allocation for makerA"))
			d.expectTradeLocked(f)
			d.expectRecorded(t, f, &settled)

			require.NoError(t, d.useCase(t).Tick(t.Context(), now))
			require.Equal(t, f.trade.ID, settled.ID)
			require.Equal(t, domain.TradeSettled, settled.Status)
			require.Empty(t, settled.UpdateID)
			require.NotNil(t, settled.SettledAt)
		})
	}
}

// Every party unallocated means an earlier settle landed: a ledger failure on the retry leaves the
// trade settling for the next tick, even past SETTLE_TIMEOUT.
func Test_Tick_RetriesAGoneSettlementWhileTheLedgerIsDown(t *testing.T) {
	d := newDeps(t)
	now := time.Now()
	f := newSettledFixture(now)
	f.trade = domain.NewTrade(f.rfq, f.won, now.Add(-11*time.Minute))
	errDown := domain.Errorf(domain.ErrUpstream, "ledger unavailable")

	d.expectAnswer(now, f, map[string]bool{taker.ID: false, makerA.ID: false, operator: false})
	d.ledger.EXPECT().Settle(Any, f.won.ID, Any).Return("", errDown)

	// No lock, no update, no release: the strict mocks fail on any of them.
	err := d.useCase(t).Tick(t.Context(), now)
	require.ErrorIs(t, err, errDown)
}

// A trade still partly allocated past SETTLE_TIMEOUT fails: no settle leaves that trace, since a
// settle consumes every allocation of the trade at once.
func Test_Tick_FailsAPartlyAllocatedTradeAfterTimeout(t *testing.T) {
	cases := []struct {
		name   string
		answer map[string]bool
	}{
		{"the taker never allocated", map[string]bool{taker.ID: false, makerA.ID: true, operator: true}},
		{"only the operator's allocation left", map[string]bool{taker.ID: false, makerA.ID: false, operator: true}},
		{"the operator not reported", map[string]bool{taker.ID: false, makerA.ID: false}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := newDeps(t)
			now := time.Now()
			f := newSettledFixture(now)
			f.trade = domain.NewTrade(f.rfq, f.won, now.Add(-11*time.Minute))

			d.expectAnswer(now, f, c.answer)
			d.expectTradeLocked(f)
			d.postgres.EXPECT().UpdateTrade(Any, mock.MatchedBy(func(tr domain.Trade) bool {
				return tr.ID == f.trade.ID && tr.Status == domain.TradeFailed && tr.Reason == "settlement timed out"
			})).Return(nil)
			d.postgres.EXPECT().UpdateQuote(Any, quoteIn(f.won.ID, domain.QuoteFailed)).Return(nil)
			d.ledger.EXPECT().CancelSettlement(Any, f.won.ID).Return(nil)
			d.events.EXPECT().Publish(Any, tradeEvent(f.trade, domain.TradeFailed), parties(taker, makerA)).Return(nil)
			d.events.EXPECT().Publish(Any, quoteEvent(f.won, domain.QuoteFailed), parties(makerA)).Return(nil)

			require.NoError(t, d.useCase(t).Tick(t.Context(), now))
		})
	}
}

// The trade failed meanwhile, say by another replica: the settle the ledger answered cannot be
// recorded. The log says so without claiming a settlement the ledger may not have made.
func Test_Tick_LogsASettlementItCannotRecord(t *testing.T) {
	cases := []struct {
		name     string
		updateID string
		err      error
	}{
		{"settled", "update-1", nil},
		{"allocations gone", "", domain.Errorf(domain.ErrSettlementGone, "no allocation for makerA")},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := newDeps(t)
			now := time.Now()
			f := newSettledFixture(now)
			f.won.Status = domain.QuoteFailed

			var buf bytes.Buffer

			previous := log.Logger
			log.Logger = zerolog.New(&buf)

			t.Cleanup(func() { log.Logger = previous })

			d.expectAllocated(now, f)
			d.ledger.EXPECT().Settle(Any, f.won.ID, Any).Return(c.updateID, c.err)

			require.NoError(t, d.useCase(t).Tick(t.Context(), now))

			logged := buf.String()
			require.Contains(t, logged, `"level":"error"`)
			require.Contains(t, logged, f.trade.ID.String())
			require.Contains(t, logged, `"update":"`+c.updateID+`"`)
			require.NotContains(t, logged, "settled on the ledger")
		})
	}
}

// A stop during one settlement: that settlement runs to its end, the next ready trade is not
// settled; it waits for the next process.
func Test_Tick_StartsNoSettlementAfterAStop(t *testing.T) {
	d := newDeps(t)
	now := time.Now()
	first, second := newSettledFixture(now), newSettledFixture(now)
	ctx, stop := context.WithCancel(t.Context())

	var settled domain.Trade

	d.expectTick(now, book{
		live:     []domain.Quote{first.sibling, second.sibling},
		settling: []domain.Trade{first.trade, second.trade},
	})
	d.cachedDesk()
	d.ledger.EXPECT().Settlements(Any, Any).Return(map[uuid.UUID]map[string]bool{
		first.won.ID:  {taker.ID: true, makerA.ID: true, operator: true},
		second.won.ID: {taker.ID: true, makerA.ID: true, operator: true},
	}, nil)
	d.ledger.EXPECT().Settle(Any, first.won.ID, Any).RunAndReturn(
		func(context.Context, uuid.UUID, []domain.Leg) (string, error) {
			stop()

			return "update-1", nil
		})
	d.postgres.EXPECT().GetRFQForUpdate(Any, first.rfq.ID).Return(first.rfq, nil)
	d.postgres.EXPECT().GetQuoteForUpdate(Any, first.won.ID).Return(first.won, nil)
	d.expectRecorded(t, first, &settled)

	// Nothing of the second trade: no Settle, no lock, no release. The strict mocks fail on any.
	err := d.useCase(t).Tick(ctx, now)
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, first.trade.ID, settled.ID)
	require.Equal(t, domain.TradeSettled, settled.Status)
}
