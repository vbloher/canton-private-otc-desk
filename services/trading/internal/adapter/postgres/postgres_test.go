//go:build integration

package postgres_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/pkg/decimal"
	"github.com/vbloher/canton-private-otc-desk/pkg/migrate"
	"github.com/vbloher/canton-private-otc-desk/pkg/otel"
	pgpool "github.com/vbloher/canton-private-otc-desk/pkg/postgres"
	"github.com/vbloher/canton-private-otc-desk/pkg/transaction"
	"github.com/vbloher/canton-private-otc-desk/services/trading/internal/adapter/postgres"
	"github.com/vbloher/canton-private-otc-desk/services/trading/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/trading/migration"
)

func connect(t *testing.T) (*postgres.Postgres, *transaction.Manager) {
	t.Helper()

	otel.SilentModeInit()

	admin := pgpool.Config{User: "otc", Password: "otc", Host: "localhost", Port: "5432", DBName: "postgres"}

	adminPool, err := pgpool.New(t.Context(), admin)
	require.NoError(t, err)
	t.Cleanup(adminPool.Close)

	c := admin
	c.DBName = "trading_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")[:12]

	_, err = adminPool.Exec(t.Context(), "CREATE DATABASE "+c.DBName)
	require.NoError(t, err)

	require.NoError(t, migrate.Up(c.URL(), migration.FS))

	pool, err := pgpool.New(t.Context(), c)
	require.NoError(t, err)

	t.Cleanup(func() {
		pool.Close()

		_, err := adminPool.Exec(context.WithoutCancel(t.Context()), "DROP DATABASE "+c.DBName+" WITH (FORCE)")
		require.NoError(t, err)
	})

	tx := transaction.New(pool)

	return postgres.New(tx), tx
}

func Test_Postgres_Book(t *testing.T) {
	pg, tx := connect(t)
	ctx := t.Context()
	now := time.Now().UTC().Truncate(time.Microsecond)
	run := uuid.NewString()[:8]
	taker := domain.Party{ID: "alice-" + run + "::1220", Hint: "alice-" + run}
	makerA := domain.Party{ID: "makerA-" + run + "::1220", Hint: "makerA"}
	makerB := domain.Party{ID: "makerB-" + run + "::1220", Hint: "makerB"}

	rfq := domain.RFQ{
		ID: uuid.New(), TakerParty: taker.ID, TakerHint: taker.Hint, Direction: domain.Sell,
		Qty: decimal.MustParse("1234567890123456789012345678.0123456789"), Base: "CBTC", Quote: "USDCx",
		Makers: []domain.Party{makerA, makerB}, FeeBps: 10, Deadline: now.Add(-time.Second),
		Status: domain.RFQOpen, CreatedAt: now.Add(-time.Minute),
	}

	require.NoError(t, pg.CreateRFQ(ctx, rfq))

	got, err := pg.GetRFQForUpdate(ctx, rfq.ID)
	require.NoError(t, err)
	require.Equal(t, rfq.Qty.String(), got.Qty.String())
	require.Equal(t, rfq.Makers, got.Makers)
	require.True(t, rfq.Deadline.Equal(got.Deadline))
	require.Equal(t, domain.Sell, got.Direction)
	require.Equal(t, domain.RFQOpen, got.Status)
	require.Equal(t, int64(10), got.FeeBps)
	require.Nil(t, got.ClosedAt)

	_, err = pg.GetRFQForUpdate(ctx, uuid.New())
	require.ErrorIs(t, err, domain.ErrNotFound)

	listed, err := pg.ListRFQs(ctx, []uuid.UUID{rfq.ID, uuid.New()})
	require.NoError(t, err)
	require.Len(t, listed, 1)

	mine, err := pg.ListTakerRFQs(ctx, taker.ID, now.Add(-time.Hour))
	require.NoError(t, err)
	require.Len(t, mine, 1)

	incoming, err := pg.ListMakerRFQs(ctx, makerB.ID, now.Add(-time.Hour))
	require.NoError(t, err)
	require.Len(t, incoming, 1)

	incoming, err = pg.ListMakerRFQs(ctx, taker.ID, now.Add(-time.Hour))
	require.NoError(t, err)
	require.Empty(t, incoming)

	expired, err := pg.ListExpiredRFQs(ctx, now)
	require.NoError(t, err)
	require.Contains(t, ids(expired), rfq.ID)

	quote := domain.Quote{
		ID: uuid.New(), RFQID: rfq.ID, Maker: makerA, Price: decimal.MustParse("100000.5"),
		ValidUntil: now.Add(time.Minute), Status: domain.QuoteAllocating, CreatedAt: now,
	}
	require.NoError(t, pg.CreateQuote(ctx, quote))

	again := quote
	again.ID = uuid.New()
	require.ErrorIs(t, pg.CreateQuote(ctx, again), domain.ErrAlreadyQuoted)

	require.NoError(t, tx.Wrap(ctx, func(ctx context.Context) error {
		locked, err := pg.GetQuoteForUpdate(ctx, quote.ID)
		require.NoError(t, err)
		require.Equal(t, "100000.5", locked.Price.String())
		require.Equal(t, makerA, locked.Maker)
		require.Nil(t, locked.OpenedAt, "an allocating quote never opened")

		return pg.UpdateQuote(ctx, locked.Open(now).WithStatus(domain.QuoteAccepted))
	}))

	read, err := pg.GetQuote(ctx, quote.ID)
	require.NoError(t, err)
	require.Equal(t, domain.QuoteAccepted, read.Status)
	require.True(t, quote.ValidUntil.Equal(read.ValidUntil))
	require.NotNil(t, read.OpenedAt)
	require.True(t, now.Equal(*read.OpenedAt))

	byStatus, err := pg.ListQuotesByStatus(ctx, domain.QuoteAllocating, domain.QuoteAccepted)
	require.NoError(t, err)
	require.Contains(t, quoteIDs(byStatus), quote.ID)

	quotes, err := pg.ListQuotes(ctx, []uuid.UUID{rfq.ID})
	require.NoError(t, err)
	require.Len(t, quotes, 1)

	trade := domain.NewTrade(rfq, read, now)
	trade.Price = decimal.MustParse("1.5")
	trade.Notional = decimal.MustParse("0.0000000001")
	trade.Fee = decimal.Zero
	require.NoError(t, pg.CreateTrade(ctx, trade))

	settling, err := pg.ListSettlingTrades(ctx)
	require.NoError(t, err)
	require.Contains(t, tradeIDs(settling), trade.ID)

	expired, err = pg.ListExpiredRFQs(ctx, now)
	require.NoError(t, err)
	require.NotContains(t, ids(expired), rfq.ID, "an rfq with a settling trade does not expire")

	require.NoError(t, pg.UpdateTrade(ctx, trade.Settled("update-1", now)))

	trades, err := pg.ListTrades(ctx, makerA.ID, 100)
	require.NoError(t, err)
	require.Len(t, trades, 1)
	require.Equal(t, domain.TradeSettled, trades[0].Status)
	require.Equal(t, "update-1", trades[0].UpdateID)
	require.Equal(t, "0.0000000001", trades[0].Notional.String())
	require.Equal(t, "0.0", trades[0].Fee.String())
	require.Equal(t, taker, trades[0].Taker)
	require.NotNil(t, trades[0].SettledAt)

	require.NoError(t, pg.UpdateRFQ(ctx, rfq.Close(domain.RFQExpired, now)))

	mine, err = pg.ListTakerRFQs(ctx, taker.ID, now.Add(-time.Hour))
	require.NoError(t, err)
	require.Len(t, mine, 1, "closed within the window")

	mine, err = pg.ListTakerRFQs(ctx, taker.ID, now.Add(time.Second))
	require.NoError(t, err)
	require.Empty(t, mine, "closed before the window")

	history, err := pg.ListRFQHistory(ctx, taker.ID, 100)
	require.NoError(t, err)
	require.Len(t, history, 1)
	require.NotNil(t, history[0].ClosedAt)

	require.ErrorIs(t, pg.UpdateRFQ(ctx, domain.RFQ{ID: uuid.New()}), domain.ErrNotFound)
}

func ids(rfqs []domain.RFQ) []uuid.UUID {
	out := make([]uuid.UUID, 0, len(rfqs))
	for _, r := range rfqs {
		out = append(out, r.ID)
	}

	return out
}

func quoteIDs(quotes []domain.Quote) []uuid.UUID {
	out := make([]uuid.UUID, 0, len(quotes))
	for _, q := range quotes {
		out = append(out, q.ID)
	}

	return out
}

func tradeIDs(trades []domain.Trade) []uuid.UUID {
	out := make([]uuid.UUID, 0, len(trades))
	for _, t := range trades {
		out = append(out, t.ID)
	}

	return out
}
