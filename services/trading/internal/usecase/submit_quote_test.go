package usecase_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/pkg/decimal"
	"github.com/vbloher/canton-private-otc-desk/pkg/events"
	"github.com/vbloher/canton-private-otc-desk/services/trading/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/trading/internal/dto"
)

func (d deps) expectQuoteSaved(rfq domain.RFQ, saved *domain.Quote) {
	d.postgres.EXPECT().GetRFQForUpdate(Any, rfq.ID).Return(rfq, nil)
	d.postgres.EXPECT().CreateQuote(Any, Any).Run(func(_ context.Context, q domain.Quote) { *saved = q }).Return(nil)
	d.events.EXPECT().Publish(Any, mock.MatchedBy(func(ev events.Event) bool {
		return ev == quoteEvent(*saved, domain.QuoteAllocating)
	}), parties(makerA)).Return(nil)
}

func Test_SubmitQuote_AllocatesAsMaker(t *testing.T) {
	d := newDeps(t)
	now := time.Now()
	rfq := openRFQ(now.Add(-time.Minute))

	var saved domain.Quote

	d.cachedDesk()
	d.expectQuoteSaved(rfq, &saved)
	d.ledger.EXPECT().Allocate(Any, Any, Any, Any, makerA.ID, Any).RunAndReturn(
		func(_ context.Context, id uuid.UUID, deadline time.Time, legs []domain.Leg, _, _ string) error {
			require.Equal(t, saved.ID, id, "the settlement id is the quote id")
			require.Equal(t, saved.ValidUntil.Add(30*time.Minute), deadline)
			require.Equal(t, show(legsOf(t, rfq, saved)), show(legs))

			return nil
		})
	d.postgres.EXPECT().GetQuoteForUpdate(Any, Any).RunAndReturn(func(context.Context, uuid.UUID) (domain.Quote, error) {
		return saved, nil
	})

	quote, err := d.useCase(t).SubmitQuote(t.Context(), dto.SubmitQuoteInput{
		Maker: makerA, RFQID: rfq.ID, Price: "100150",
	})
	require.NoError(t, err)
	require.Equal(t, saved, quote)
	require.Equal(t, domain.QuoteAllocating, quote.Status)
	require.Equal(t, rfq.ID, quote.RFQID)
	require.Equal(t, makerA, quote.Maker)
	require.Equal(t, "100150.0", quote.Price.String())
	require.WithinDuration(t, time.Now().Add(time.Minute), quote.ValidUntil, 5*time.Second)
}

func Test_SubmitQuote_AlreadyQuoted(t *testing.T) {
	d := newDeps(t)
	rfq := openRFQ(time.Now().Add(-time.Minute))

	d.cachedDesk()
	d.postgres.EXPECT().GetRFQForUpdate(Any, rfq.ID).Return(rfq, nil)
	d.postgres.EXPECT().CreateQuote(Any, Any).Return(domain.ErrAlreadyQuoted)

	_, err := d.useCase(t).SubmitQuote(t.Context(), dto.SubmitQuoteInput{
		Maker: makerA, RFQID: rfq.ID, Price: "100150",
	})
	require.ErrorIs(t, err, domain.ErrAlreadyQuoted)
}

func Test_SubmitQuote_InsufficientFunds(t *testing.T) {
	d := newDeps(t)
	rfq := openRFQ(time.Now().Add(-time.Minute))
	ttl := 30

	var saved, failed domain.Quote

	d.cachedDesk()
	d.expectQuoteSaved(rfq, &saved)
	d.ledger.EXPECT().Allocate(Any, Any, Any, Any, makerA.ID, Any).
		Return(domain.Errorf(domain.ErrInsufficientFunds, "need 0.5 CBTC, held 0.1"))
	d.postgres.EXPECT().GetQuoteForUpdate(Any, Any).RunAndReturn(
		func(_ context.Context, id uuid.UUID) (domain.Quote, error) {
			require.Equal(t, saved.ID, id)

			return saved, nil
		})
	d.postgres.EXPECT().UpdateQuote(Any, Any).Run(func(_ context.Context, q domain.Quote) { failed = q }).Return(nil)
	d.ledger.EXPECT().CancelSettlement(Any, Any).RunAndReturn(func(_ context.Context, id uuid.UUID) error {
		require.Equal(t, saved.ID, id)

		return nil
	})
	d.events.EXPECT().Publish(Any, mock.MatchedBy(func(ev events.Event) bool {
		return ev == quoteEvent(saved, domain.QuoteFailed)
	}), parties(makerA)).Return(nil)

	_, err := d.useCase(t).SubmitQuote(t.Context(), dto.SubmitQuoteInput{
		Maker: makerA, RFQID: rfq.ID, Price: "100150", TTLSeconds: &ttl,
	})
	require.ErrorIs(t, err, domain.ErrInsufficientFunds)
	require.Equal(t, saved.ID, failed.ID)
	require.Equal(t, domain.QuoteFailed, failed.Status, "the taker never sees a failed quote")
}

func Test_SubmitQuote_DustRejected(t *testing.T) {
	d := newDeps(t)
	rfq := openRFQ(time.Now().Add(-time.Minute))
	rfq.Qty = decimal.MustParse("0.0000000001")

	d.cachedDesk()
	d.postgres.EXPECT().GetRFQForUpdate(Any, rfq.ID).Return(rfq, nil)

	_, err := d.useCase(t).SubmitQuote(t.Context(), dto.SubmitQuoteInput{
		Maker: makerA, RFQID: rfq.ID, Price: "0.5",
	})
	require.ErrorIs(t, err, domain.ErrDust)
}

func Test_SubmitQuote_NotInvited(t *testing.T) {
	d := newDeps(t)
	rfq := openRFQ(time.Now().Add(-time.Minute))
	stranger := domain.Party{ID: "bob::1220", Hint: "bob"}

	d.cachedDesk()
	d.postgres.EXPECT().GetRFQForUpdate(Any, rfq.ID).Return(rfq, nil)

	_, err := d.useCase(t).SubmitQuote(t.Context(), dto.SubmitQuoteInput{
		Maker: stranger, RFQID: rfq.ID, Price: "100150",
	})
	require.ErrorIs(t, err, domain.ErrForbidden)
}

func Test_SubmitQuote_ReleasesAQuoteClosedDuringItsAllocation(t *testing.T) {
	cases := []struct {
		now     domain.QuoteStatus
		release bool
	}{
		{domain.QuoteCancelled, true},
		{domain.QuoteExpired, true},
		{domain.QuoteOpen, false},
		{domain.QuoteAccepted, false},
	}

	for _, c := range cases {
		t.Run(string(c.now), func(t *testing.T) {
			d := newDeps(t)
			rfq := openRFQ(time.Now().Add(-time.Minute))

			var saved domain.Quote

			d.cachedDesk()
			d.expectQuoteSaved(rfq, &saved)
			d.ledger.EXPECT().Allocate(Any, Any, Any, Any, makerA.ID, Any).Return(nil)
			d.postgres.EXPECT().GetQuoteForUpdate(Any, Any).RunAndReturn(
				func(context.Context, uuid.UUID) (domain.Quote, error) {
					return saved.WithStatus(c.now), nil
				})

			if c.release {
				d.ledger.EXPECT().CancelSettlement(Any, Any).RunAndReturn(func(_ context.Context, id uuid.UUID) error {
					require.Equal(t, saved.ID, id)

					return nil
				})
			}

			quote, err := d.useCase(t).SubmitQuote(t.Context(), dto.SubmitQuoteInput{
				Maker: makerA, RFQID: rfq.ID, Price: "100150",
			})
			require.NoError(t, err)
			require.Equal(t, saved.ID, quote.ID)
			require.Equal(t, c.now, quote.Status, "the answer shows the quote as it is now")
		})
	}
}

func Test_SubmitQuote_FailedAllocationReleasesOnlyADeadQuote(t *testing.T) {
	cases := []struct {
		now     domain.QuoteStatus
		release bool
	}{
		{domain.QuoteOpen, false},
		{domain.QuoteAccepted, false},
		{domain.QuoteCancelled, true},
	}

	for _, c := range cases {
		t.Run(string(c.now), func(t *testing.T) {
			d := newDeps(t)
			rfq := openRFQ(time.Now().Add(-time.Minute))

			var saved domain.Quote

			d.cachedDesk()
			d.expectQuoteSaved(rfq, &saved)
			d.ledger.EXPECT().Allocate(Any, Any, Any, Any, makerA.ID, Any).
				Return(domain.Errorf(domain.ErrUpstream, "deadline exceeded"))
			d.postgres.EXPECT().GetQuoteForUpdate(Any, Any).RunAndReturn(
				func(context.Context, uuid.UUID) (domain.Quote, error) {
					return saved.WithStatus(c.now), nil
				})

			if c.release {
				d.ledger.EXPECT().CancelSettlement(Any, Any).RunAndReturn(func(_ context.Context, id uuid.UUID) error {
					require.Equal(t, saved.ID, id)

					return nil
				})
			}

			_, err := d.useCase(t).SubmitQuote(t.Context(), dto.SubmitQuoteInput{
				Maker: makerA, RFQID: rfq.ID, Price: "100150",
			})
			require.ErrorIs(t, err, domain.ErrUpstream)
		})
	}
}
