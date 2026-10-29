package usecase_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/pkg/events"
	"github.com/vbloher/canton-private-otc-desk/services/trading/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/trading/internal/dto"
)

type acceptFixture struct {
	rfq     domain.RFQ
	quote   domain.Quote
	sibling domain.Quote
}

func newAcceptFixture(now time.Time) acceptFixture {
	rfq := openRFQ(now.Add(-time.Minute))

	return acceptFixture{
		rfq:     rfq,
		quote:   quoteOf(rfq, makerA, "100000", domain.QuoteOpen, now.Add(time.Minute)),
		sibling: quoteOf(rfq, makerB, "100100", domain.QuoteOpen, now.Add(time.Minute)),
	}
}

func (d deps) expectLocked(rfq domain.RFQ, quote domain.Quote) *mock.Call {
	read := d.postgres.EXPECT().GetQuote(Any, quote.ID).Return(quote, nil).Call
	lockRFQ := d.postgres.EXPECT().GetRFQForUpdate(Any, rfq.ID).Return(rfq, nil).NotBefore(read)
	d.postgres.EXPECT().GetQuoteForUpdate(Any, quote.ID).Return(quote, nil).Once().NotBefore(lockRFQ)

	return lockRFQ
}

func (d deps) expectAccepted(f acceptFixture, trade *domain.Trade) *mock.Call {
	d.cachedDesk()
	lockRFQ := d.expectLocked(f.rfq, f.quote)
	d.postgres.EXPECT().ListQuotes(Any, []uuid.UUID{f.rfq.ID}).Return([]domain.Quote{f.quote, f.sibling}, nil)
	d.postgres.EXPECT().UpdateQuote(Any, quoteIn(f.quote.ID, domain.QuoteAccepted)).Return(nil)
	d.postgres.EXPECT().CreateTrade(Any, Any).Run(func(_ context.Context, tr domain.Trade) { *trade = tr }).Return(nil)
	d.events.EXPECT().Publish(Any, quoteEvent(f.quote, domain.QuoteAccepted), parties(taker, makerA)).Return(nil)
	d.events.EXPECT().Publish(Any, mock.MatchedBy(func(ev events.Event) bool {
		return ev == tradeEvent(*trade, domain.TradeSettling)
	}), parties(taker, makerA)).Return(nil)

	return lockRFQ
}

func Test_AcceptQuote_AllocatesTakerAndOperator(t *testing.T) {
	d := newDeps(t)
	f := newAcceptFixture(time.Now())

	var trade domain.Trade

	d.expectAccepted(f, &trade)

	legs := legsOf(t, f.rfq, f.quote)
	deadline := at(f.quote.ValidUntil.Add(30 * time.Minute))
	operatorCall := d.ledger.EXPECT().Allocate(Any, f.quote.ID, deadline, sameLegs(legs), operator, Any).Return(nil).Once()
	d.ledger.EXPECT().Allocate(Any, f.quote.ID, deadline, sameLegs(legs), taker.ID, Any).Return(nil).Once().
		NotBefore(operatorCall)

	out, err := d.useCase(t).AcceptQuote(t.Context(), dto.AcceptQuoteInput{Taker: taker, QuoteID: f.quote.ID})
	require.NoError(t, err)
	require.Equal(t, trade.ID, out.ID)
	require.Equal(t, domain.TradeSettling, out.Status)
	require.Equal(t, f.quote.ID, out.QuoteID)
	require.Equal(t, f.rfq.ID, out.RFQID)
	require.Equal(t, taker, out.Taker)
	require.Equal(t, makerA, out.Maker)
	require.Equal(t, "50000.0", out.Notional.String())
	require.Equal(t, "50.0", out.Fee.String())
}

func Test_AcceptQuote_NoFeeAllocatesTakerOnly(t *testing.T) {
	d := newDeps(t)
	f := newAcceptFixture(time.Now())
	f.rfq.FeeBps = 0

	var trade domain.Trade

	d.expectAccepted(f, &trade)
	d.ledger.EXPECT().Allocate(Any, f.quote.ID, Any, Any, taker.ID, Any).Return(nil).Once()

	out, err := d.useCase(t).AcceptQuote(t.Context(), dto.AcceptQuoteInput{Taker: taker, QuoteID: f.quote.ID})
	require.NoError(t, err)
	require.Equal(t, "0.0", out.Fee.String())
}

func Test_AcceptQuote_NotOpen(t *testing.T) {
	now := time.Now()

	cases := []struct {
		name   string
		modify func(q *domain.Quote)
	}{
		{"accepted twice", func(q *domain.Quote) { q.Status = domain.QuoteAccepted }},
		{"after validUntil", func(q *domain.Quote) { q.ValidUntil = now.Add(-time.Second) }},
		{"still allocating", func(q *domain.Quote) { q.Status = domain.QuoteAllocating }},
		{"lost", func(q *domain.Quote) { q.Status = domain.QuoteLost }},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := newDeps(t)
			f := newAcceptFixture(now)
			c.modify(&f.quote)

			d.cachedDesk()
			d.expectLocked(f.rfq, f.quote)

			_, err := d.useCase(t).AcceptQuote(t.Context(), dto.AcceptQuoteInput{Taker: taker, QuoteID: f.quote.ID})
			require.ErrorIs(t, err, domain.ErrQuoteNotOpen)
		})
	}
}

func Test_AcceptQuote_OneSettlementPerRFQ(t *testing.T) {
	d := newDeps(t)
	f := newAcceptFixture(time.Now())
	f.sibling.Status = domain.QuoteAccepted

	d.cachedDesk()
	d.expectLocked(f.rfq, f.quote)
	d.postgres.EXPECT().ListQuotes(Any, []uuid.UUID{f.rfq.ID}).Return([]domain.Quote{f.quote, f.sibling}, nil)

	_, err := d.useCase(t).AcceptQuote(t.Context(), dto.AcceptQuoteInput{Taker: taker, QuoteID: f.quote.ID})
	require.ErrorIs(t, err, domain.ErrRFQClosed)
}

func Test_AcceptQuote_NotTheTaker(t *testing.T) {
	d := newDeps(t)
	f := newAcceptFixture(time.Now())

	d.cachedDesk()
	d.expectLocked(f.rfq, f.quote)

	_, err := d.useCase(t).AcceptQuote(t.Context(), dto.AcceptQuoteInput{Taker: makerB, QuoteID: f.quote.ID})
	require.ErrorIs(t, err, domain.ErrForbidden)
}

func (d deps) expectTradeFailed(f acceptFixture, lockRFQ *mock.Call, trade, failed *domain.Trade) {
	d.postgres.EXPECT().GetQuoteForUpdate(Any, f.quote.ID).Return(f.quote.WithStatus(domain.QuoteAccepted), nil).
		Once().NotBefore(lockRFQ)
	d.postgres.EXPECT().UpdateTrade(Any, Any).Run(func(_ context.Context, tr domain.Trade) { *failed = tr }).Return(nil)
	d.postgres.EXPECT().UpdateQuote(Any, quoteIn(f.quote.ID, domain.QuoteFailed)).Return(nil)
	d.ledger.EXPECT().CancelSettlement(Any, f.quote.ID).Return(nil)
	d.events.EXPECT().Publish(Any, mock.MatchedBy(func(ev events.Event) bool {
		return ev == tradeEvent(*trade, domain.TradeFailed)
	}), parties(taker, makerA)).Return(nil)
	d.events.EXPECT().Publish(Any, quoteEvent(f.quote, domain.QuoteFailed), parties(makerA)).Return(nil)
}

func Test_AcceptQuote_AllocationFails(t *testing.T) {
	d := newDeps(t)
	f := newAcceptFixture(time.Now())

	var trade, failedTrade domain.Trade

	lockRFQ := d.expectAccepted(f, &trade)
	operatorCall := d.ledger.EXPECT().Allocate(Any, f.quote.ID, Any, Any, operator, Any).Return(nil).Once()
	d.ledger.EXPECT().Allocate(Any, f.quote.ID, Any, Any, taker.ID, Any).
		Return(domain.Errorf(domain.ErrInsufficientFunds, "need 50000.0 USDCx, held 10.0")).Once().NotBefore(operatorCall)
	d.expectTradeFailed(f, lockRFQ, &trade, &failedTrade)

	_, err := d.useCase(t).AcceptQuote(t.Context(), dto.AcceptQuoteInput{Taker: taker, QuoteID: f.quote.ID})
	require.ErrorIs(t, err, domain.ErrInsufficientFunds)
	require.Equal(t, trade.ID, failedTrade.ID)
	require.Equal(t, domain.TradeFailed, failedTrade.Status)
	require.Equal(t, "insufficient funds", failedTrade.Reason)
}

func Test_AcceptQuote_OperatorFailureLeavesTheTakerAlone(t *testing.T) {
	d := newDeps(t)
	f := newAcceptFixture(time.Now())

	var trade, failedTrade domain.Trade

	lockRFQ := d.expectAccepted(f, &trade)
	d.ledger.EXPECT().Allocate(Any, f.quote.ID, Any, Any, operator, Any).
		Return(domain.Errorf(domain.ErrUpstream, "ledger unavailable")).Once()
	d.expectTradeFailed(f, lockRFQ, &trade, &failedTrade)

	_, err := d.useCase(t).AcceptQuote(t.Context(), dto.AcceptQuoteInput{Taker: taker, QuoteID: f.quote.ID})
	require.ErrorIs(t, err, domain.ErrUpstream)
	require.Equal(t, domain.TradeFailed, failedTrade.Status)
	require.Equal(t, "allocation failed", failedTrade.Reason)
}

func Test_AcceptQuote_ChecksTheClockAfterLocking(t *testing.T) {
	d := newDeps(t)
	f := newAcceptFixture(time.Now())
	f.quote.ValidUntil = time.Now().Add(20 * time.Millisecond)

	d.cachedDesk()
	d.postgres.EXPECT().GetQuote(Any, f.quote.ID).Return(f.quote, nil)
	d.postgres.EXPECT().GetRFQForUpdate(Any, f.rfq.ID).Return(f.rfq, nil)
	d.postgres.EXPECT().GetQuoteForUpdate(Any, f.quote.ID).RunAndReturn(
		func(context.Context, uuid.UUID) (domain.Quote, error) {
			time.Sleep(60 * time.Millisecond)

			return f.quote, nil
		})

	_, err := d.useCase(t).AcceptQuote(t.Context(), dto.AcceptQuoteInput{Taker: taker, QuoteID: f.quote.ID})
	require.ErrorIs(t, err, domain.ErrQuoteNotOpen)
}
