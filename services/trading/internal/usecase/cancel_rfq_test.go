package usecase_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/services/trading/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/trading/internal/dto"
)

func Test_CancelRFQ_CancelsLiveQuotes(t *testing.T) {
	d := newDeps(t)
	now := time.Now()
	rfq := openRFQ(now.Add(-time.Minute))
	open := quoteOf(rfq, makerA, "100000", domain.QuoteOpen, now.Add(time.Minute))
	allocating := quoteOf(rfq, makerB, "100100", domain.QuoteAllocating, now.Add(time.Minute))
	failed := quoteOf(rfq, makerC, "100200", domain.QuoteFailed, now.Add(time.Minute))

	d.postgres.EXPECT().GetRFQForUpdate(Any, rfq.ID).Return(rfq, nil)
	d.postgres.EXPECT().ListQuotes(Any, []uuid.UUID{rfq.ID}).Return([]domain.Quote{open, allocating, failed}, nil)

	var cancelled domain.RFQ

	d.postgres.EXPECT().UpdateRFQ(Any, Any).Run(func(_ context.Context, r domain.RFQ) { cancelled = r }).Return(nil)

	for _, q := range []domain.Quote{open, allocating} {
		d.postgres.EXPECT().UpdateQuote(Any, quoteIn(q.ID, domain.QuoteCancelled)).Return(nil)
		d.ledger.EXPECT().CancelSettlement(Any, q.ID).Return(nil)
	}

	d.events.EXPECT().Publish(Any, quoteEvent(open, domain.QuoteCancelled), parties(taker, makerA)).Return(nil)
	d.events.EXPECT().Publish(Any, quoteEvent(allocating, domain.QuoteCancelled), parties(makerB)).Return(nil)

	d.events.EXPECT().Publish(Any, rfqUpdated(rfq), parties(taker, makerA, makerB, makerC)).Return(nil)

	err := d.useCase(t).CancelRFQ(t.Context(), dto.CancelRFQInput{Taker: taker, RFQID: rfq.ID})
	require.NoError(t, err)
	require.Equal(t, domain.RFQCancelled, cancelled.Status)
	require.NotNil(t, cancelled.ClosedAt)
}

func Test_CancelRFQ_LedgerFailureDoesNotUndoTheCancel(t *testing.T) {
	d := newDeps(t)
	now := time.Now()
	rfq := openRFQ(now.Add(-time.Minute))
	open := quoteOf(rfq, makerA, "100000", domain.QuoteOpen, now.Add(time.Minute))

	d.postgres.EXPECT().GetRFQForUpdate(Any, rfq.ID).Return(rfq, nil)
	d.postgres.EXPECT().ListQuotes(Any, []uuid.UUID{rfq.ID}).Return([]domain.Quote{open}, nil)
	d.postgres.EXPECT().UpdateRFQ(Any, Any).Return(nil)
	d.postgres.EXPECT().UpdateQuote(Any, quoteIn(open.ID, domain.QuoteCancelled)).Return(nil)
	d.ledger.EXPECT().CancelSettlement(Any, open.ID).Return(domain.Errorf(domain.ErrUpstream, "ledger unavailable"))
	d.events.EXPECT().Publish(Any, Any, Any).Return(nil).Twice()

	err := d.useCase(t).CancelRFQ(t.Context(), dto.CancelRFQInput{Taker: taker, RFQID: rfq.ID})
	require.NoError(t, err)
}

func Test_CancelRFQ_Rejects(t *testing.T) {
	now := time.Now()
	base := openRFQ(now.Add(-time.Minute))

	cases := []struct {
		name   string
		caller domain.Party
		rfq    domain.RFQ
		quotes []domain.Quote
		err    error
	}{
		{"not the taker", makerA, base, nil, domain.ErrForbidden},
		{"already closed", taker, base.Close(domain.RFQExpired, now), nil, domain.ErrRFQClosed},
		{"trade settling", taker, base, []domain.Quote{
			quoteOf(base, makerA, "100000", domain.QuoteAccepted, now.Add(time.Minute)),
		}, domain.ErrRFQClosed},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := newDeps(t)
			d.postgres.EXPECT().GetRFQForUpdate(Any, c.rfq.ID).Return(c.rfq, nil)

			if c.quotes != nil {
				d.postgres.EXPECT().ListQuotes(Any, []uuid.UUID{c.rfq.ID}).Return(c.quotes, nil)
			}

			err := d.useCase(t).CancelRFQ(t.Context(), dto.CancelRFQInput{Taker: c.caller, RFQID: c.rfq.ID})
			require.ErrorIs(t, err, c.err)
		})
	}
}

func Test_CancelRFQ_ReleasesAfterTheCallerLeft(t *testing.T) {
	d := newDeps(t)
	now := time.Now()
	rfq := openRFQ(now.Add(-time.Minute))
	open := quoteOf(rfq, makerA, "100000", domain.QuoteOpen, now.Add(time.Minute))
	ctx, leave := context.WithCancel(t.Context())
	live := mock.MatchedBy(func(ctx context.Context) bool { return ctx.Err() == nil })

	d.postgres.EXPECT().GetRFQForUpdate(Any, rfq.ID).Return(rfq, nil)
	d.postgres.EXPECT().ListQuotes(Any, []uuid.UUID{rfq.ID}).Return([]domain.Quote{open}, nil)
	d.postgres.EXPECT().UpdateRFQ(Any, Any).Return(nil)
	d.postgres.EXPECT().UpdateQuote(Any, quoteIn(open.ID, domain.QuoteCancelled)).Return(nil)
	d.ledger.EXPECT().CancelSettlement(live, open.ID).Return(nil)
	d.events.EXPECT().Publish(live, Any, Any).Return(nil).Twice()

	err := d.useCaseLeftAfterCommit(t, leave).CancelRFQ(ctx, dto.CancelRFQInput{Taker: taker, RFQID: rfq.ID})
	require.NoError(t, err)
}
