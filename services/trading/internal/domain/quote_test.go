package domain_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/pkg/decimal"
	"github.com/vbloher/canton-private-otc-desk/services/trading/internal/domain"
)

var maker = domain.Party{ID: makerA, Hint: "makerA"}

func openRFQ(now time.Time, ttl time.Duration) domain.RFQ {
	rfq, err := domain.NewRFQ(domain.RFQParams{
		Taker:     domain.Party{ID: taker, Hint: "alice"},
		Direction: domain.Buy,
		Qty:       decimal.MustParse("0.5"),
		Base:      "CBTC",
		Quote:     "USDCx",
		Makers:    []domain.Party{maker},
		TTL:       ttl,
		FeeBps:    10,
	}, desk, now)
	if err != nil {
		panic(err)
	}

	return rfq
}

func Test_NewQuote_ClampsToDeadline(t *testing.T) {
	now := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	rfq := openRFQ(now.Add(-time.Minute), 90*time.Second)

	quote, err := domain.NewQuote(rfq, maker, decimal.MustParse("100000"), 60*time.Second, now)
	require.NoError(t, err)
	require.Equal(t, rfq.Deadline, quote.ValidUntil)
	require.Equal(t, now.Add(30*time.Second), quote.ValidUntil)
	require.Equal(t, rfq.ID, quote.RFQID)
	require.Equal(t, maker, quote.Maker)
	require.Equal(t, "100000.0", quote.Price.String())
	require.Equal(t, domain.QuoteAllocating, quote.Status)
	require.Equal(t, now, quote.CreatedAt)
	require.NotZero(t, quote.ID)
	require.Equal(t, rfq.Deadline.Add(30*time.Minute), quote.SettlementDeadline())
}

func Test_Times_KeepMicroseconds(t *testing.T) {
	now := time.Date(2026, 10, 21, 12, 0, 0, 123456789, time.UTC)
	rfq := openRFQ(now, time.Hour)

	quote, err := domain.NewQuote(rfq, maker, decimal.MustParse("1"), 0, now)
	require.NoError(t, err)

	trade := domain.NewTrade(rfq, quote, now)

	for _, at := range []time.Time{rfq.CreatedAt, rfq.Deadline, quote.CreatedAt, quote.ValidUntil, trade.CreatedAt} {
		require.Equal(t, 123456000, at.Nanosecond())
	}
}

func Test_NewQuote_DefaultTTL(t *testing.T) {
	now := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	rfq := openRFQ(now, time.Hour)

	quote, err := domain.NewQuote(rfq, maker, decimal.MustParse("1"), 0, now)
	require.NoError(t, err)
	require.Equal(t, now.Add(60*time.Second), quote.ValidUntil)

	quote, err = domain.NewQuote(rfq, maker, decimal.MustParse("1"), 600*time.Second, now)
	require.NoError(t, err)
	require.Equal(t, now.Add(10*time.Minute), quote.ValidUntil)
}

func Test_NewQuote_Rejects(t *testing.T) {
	now := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	price := decimal.MustParse("100000")

	closed := openRFQ(now, time.Hour)
	closed.Status = domain.RFQCancelled

	cases := []struct {
		name  string
		rfq   domain.RFQ
		maker domain.Party
		price decimal.Decimal
		ttl   time.Duration
		err   error
	}{
		{"ttl 9", openRFQ(now, time.Hour), maker, price, 9 * time.Second, domain.ErrValidation},
		{"ttl 601", openRFQ(now, time.Hour), maker, price, 601 * time.Second, domain.ErrValidation},
		{"zero price", openRFQ(now, time.Hour), maker, decimal.Zero, 0, domain.ErrValidation},
		{"rfq not open", closed, maker, price, 0, domain.ErrRFQClosed},
		{"rfq past deadline", openRFQ(now.Add(-time.Hour), time.Hour), maker, price, 0, domain.ErrRFQClosed},
		{"not invited", openRFQ(now, time.Hour), domain.Party{ID: makerB, Hint: "makerB"}, price, 0, domain.ErrForbidden},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := domain.NewQuote(c.rfq, c.maker, c.price, c.ttl, now)
			require.ErrorIs(t, err, c.err)
		})
	}
}

func Test_QuoteStatus_LiveAndDead(t *testing.T) {
	require.True(t, domain.QuoteAllocating.Live())
	require.True(t, domain.QuoteOpen.Live())
	require.False(t, domain.QuoteAccepted.Live())
	require.False(t, domain.QuoteExpired.Live())

	for _, s := range []domain.QuoteStatus{domain.QuoteExpired, domain.QuoteCancelled, domain.QuoteLost, domain.QuoteFailed} {
		require.True(t, s.Dead(), s)
	}

	for _, s := range []domain.QuoteStatus{domain.QuoteAllocating, domain.QuoteOpen, domain.QuoteAccepted, domain.QuoteWon} {
		require.False(t, s.Dead(), s)
	}
}

func Test_Quote_OpenRecordsWhen(t *testing.T) {
	at := time.Date(2026, 10, 21, 12, 0, 1, 123456789, time.UTC)
	quote := domain.Quote{Status: domain.QuoteAllocating}

	opened := quote.Open(at)
	require.Equal(t, domain.QuoteOpen, opened.Status)
	require.NotNil(t, opened.OpenedAt)
	require.Equal(t, at.Truncate(time.Microsecond), *opened.OpenedAt)
	require.Nil(t, quote.OpenedAt, "the original is left alone")
}

func Test_Quote_VisibleToTaker(t *testing.T) {
	at := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	allocating := domain.Quote{Status: domain.QuoteAllocating}
	opened := allocating.Open(at)

	require.False(t, allocating.VisibleToTaker())
	require.True(t, opened.VisibleToTaker())

	for _, status := range []domain.QuoteStatus{
		domain.QuoteAccepted, domain.QuoteWon, domain.QuoteLost, domain.QuoteExpired, domain.QuoteCancelled,
	} {
		require.True(t, opened.WithStatus(status).VisibleToTaker(), status)
		require.False(t, allocating.WithStatus(status).VisibleToTaker(), "never opened, then %s", status)
	}

	require.False(t, opened.WithStatus(domain.QuoteFailed).VisibleToTaker())
	require.False(t, allocating.WithStatus(domain.QuoteFailed).VisibleToTaker())
}

func Test_Quote_AcceptableUntilValidUntil(t *testing.T) {
	until := time.Date(2026, 10, 21, 12, 1, 0, 0, time.UTC)
	quote := domain.Quote{Status: domain.QuoteOpen, ValidUntil: until}

	require.True(t, quote.Acceptable(until.Add(-time.Microsecond)))
	require.False(t, quote.Acceptable(until), "at validUntil")
	require.False(t, quote.Acceptable(until.Add(time.Microsecond)))
	require.False(t, quote.WithStatus(domain.QuoteAllocating).Acceptable(until.Add(-time.Second)))
	require.False(t, quote.WithStatus(domain.QuoteAccepted).Acceptable(until.Add(-time.Second)))
}
