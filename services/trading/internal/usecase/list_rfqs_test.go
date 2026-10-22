package usecase_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/services/trading/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/trading/internal/dto"
)

// anHourAgo matches the start of the one-hour "recent" window.
func anHourAgo() any {
	return mock.MatchedBy(func(since time.Time) bool {
		return time.Since(since) >= time.Hour && time.Since(since) < time.Hour+time.Minute
	})
}

func Test_ListMyRFQs_HidesQuotesThatNeverLanded(t *testing.T) {
	d := newDeps(t)
	now := time.Now()
	rfq := openRFQ(now.Add(-time.Minute))
	open := quoteOf(rfq, makerA, "100000", domain.QuoteOpen, now.Add(time.Minute))
	allocating := quoteOf(rfq, makerB, "100100", domain.QuoteAllocating, now.Add(time.Minute))
	failed := quoteOf(rfq, makerC, "100200", domain.QuoteFailed, now.Add(time.Minute))
	quiet := openRFQ(now.Add(-2 * time.Minute))
	lapsed := quoteOf(quiet, makerA, "100300", domain.QuoteExpired, now.Add(-time.Minute))
	neverLanded := neverOpened(quoteOf(quiet, makerB, "99000", domain.QuoteExpired, now.Add(-time.Minute)))

	d.postgres.EXPECT().ListTakerRFQs(Any, taker.ID, anHourAgo()).Return([]domain.RFQ{rfq, quiet}, nil)
	d.postgres.EXPECT().ListQuotes(Any, []uuid.UUID{rfq.ID, quiet.ID}).
		Return([]domain.Quote{open, allocating, failed, lapsed, neverLanded}, nil)

	out, err := d.useCase(t).ListMyRFQs(t.Context(), taker)
	require.NoError(t, err)
	require.Equal(t, []dto.RFQWithQuotes{
		{RFQ: rfq, Quotes: []domain.Quote{open}},
		{RFQ: quiet, Quotes: []domain.Quote{lapsed}}, // the price of a quote that never opened stays private
	}, out)
}

func Test_ListMyRFQs_Empty(t *testing.T) {
	d := newDeps(t)
	d.postgres.EXPECT().ListTakerRFQs(Any, taker.ID, anHourAgo()).Return(nil, nil)

	out, err := d.useCase(t).ListMyRFQs(t.Context(), taker)
	require.NoError(t, err)
	require.Empty(t, out)
}

func Test_ListIncomingRFQs_ShowsOnlyTheMakersQuote(t *testing.T) {
	d := newDeps(t)
	now := time.Now()
	quoted := openRFQ(now.Add(-time.Minute))
	unquoted := openRFQ(now.Add(-2 * time.Minute))
	mine := quoteOf(quoted, makerA, "100000", domain.QuoteAllocating, now.Add(time.Minute))
	theirs := quoteOf(quoted, makerB, "100100", domain.QuoteOpen, now.Add(time.Minute))

	d.postgres.EXPECT().ListMakerRFQs(Any, makerA.ID, anHourAgo()).Return([]domain.RFQ{quoted, unquoted}, nil)
	d.postgres.EXPECT().ListQuotes(Any, []uuid.UUID{quoted.ID, unquoted.ID}).Return([]domain.Quote{theirs, mine}, nil)

	out, err := d.useCase(t).ListIncomingRFQs(t.Context(), makerA)
	require.NoError(t, err)
	require.Equal(t, []dto.IncomingRFQ{
		{RFQ: quoted, MyQuote: &mine},
		{RFQ: unquoted},
	}, out)
}

func Test_ListRFQHistory_AndTrades_UseTheLimit(t *testing.T) {
	d := newDeps(t)
	rfq := openRFQ(time.Now())
	trade := domain.NewTrade(rfq, quoteOf(rfq, makerA, "1", domain.QuoteWon, time.Now()), time.Now())

	d.postgres.EXPECT().ListRFQHistory(Any, taker.ID, 100).Return([]domain.RFQ{rfq}, nil)
	d.postgres.EXPECT().ListTrades(Any, taker.ID, 100).Return([]domain.Trade{trade}, nil)

	history, err := d.useCase(t).ListRFQHistory(t.Context(), taker)
	require.NoError(t, err)
	require.Equal(t, []domain.RFQ{rfq}, history)

	trades, err := d.useCase(t).ListTrades(t.Context(), taker)
	require.NoError(t, err)
	require.Equal(t, []domain.Trade{trade}, trades)
}
