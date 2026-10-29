package usecase_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/pkg/decimal"
	"github.com/vbloher/canton-private-otc-desk/pkg/events"
	"github.com/vbloher/canton-private-otc-desk/pkg/otel"
	"github.com/vbloher/canton-private-otc-desk/services/trading/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/trading/internal/dto"
	"github.com/vbloher/canton-private-otc-desk/services/trading/internal/usecase"
	"github.com/vbloher/canton-private-otc-desk/services/trading/internal/usecase/mocks"
)

const (
	Any      = mock.Anything
	operator = "operator::1220"
)

var (
	taker  = domain.Party{ID: "alice::1220", Hint: "alice"}
	makerA = domain.Party{ID: "makerA::1220", Hint: "makerA"}
	makerB = domain.Party{ID: "makerB::1220", Hint: "makerB"}
	makerC = domain.Party{ID: "makerC::1220", Hint: "makerC"}
	desk   = domain.Desk{Operator: operator, Instruments: []string{"CBTC", "USDCx", "CETH"}}
	config = usecase.Config{FeeBps: 10, SettleTimeout: 10 * time.Minute}
)

type deps struct {
	postgres *mocks.Postgres
	ledger   *mocks.Ledger
	identity *mocks.Identity
	redis    *mocks.Redis
	events   *mocks.Events
}

func newDeps(t *testing.T) deps {
	t.Helper()

	otel.SilentModeInit()

	return deps{
		postgres: mocks.NewPostgres(t),
		ledger:   mocks.NewLedger(t),
		identity: mocks.NewIdentity(t),
		redis:    mocks.NewRedis(t),
		events:   mocks.NewEvents(t),
	}
}

func (d deps) useCase(t *testing.T) *usecase.UseCase {
	t.Helper()

	tx := mocks.NewTransactor(t)
	tx.EXPECT().Wrap(Any, Any).RunAndReturn(func(ctx context.Context, fn func(context.Context) error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		return fn(ctx)
	}).Maybe()

	return usecase.New(config, tx, d.postgres, d.ledger, d.identity, d.redis, d.events)
}

func (d deps) useCaseLeftAfterCommit(t *testing.T, leave context.CancelFunc) *usecase.UseCase {
	t.Helper()

	tx := mocks.NewTransactor(t)
	tx.EXPECT().Wrap(Any, Any).RunAndReturn(func(ctx context.Context, fn func(context.Context) error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		err := fn(ctx)
		leave()

		return err
	}).Maybe()

	return usecase.New(config, tx, d.postgres, d.ledger, d.identity, d.redis, d.events)
}

func (d deps) cachedDesk() {
	d.redis.EXPECT().GetDesk(Any).Return(desk, true)
}

func openRFQ(created time.Time) domain.RFQ {
	return domain.RFQ{
		ID:         uuid.New(),
		TakerParty: taker.ID,
		TakerHint:  taker.Hint,
		Direction:  domain.Buy,
		Qty:        decimal.MustParse("0.5"),
		Base:       "CBTC",
		Quote:      "USDCx",
		Makers:     []domain.Party{makerA, makerB, makerC},
		FeeBps:     10,
		Deadline:   created.Add(5 * time.Minute),
		Status:     domain.RFQOpen,
		CreatedAt:  created,
	}
}

func quoteOf(
	rfq domain.RFQ, maker domain.Party, price string, status domain.QuoteStatus, validUntil time.Time,
) domain.Quote {
	q := domain.Quote{
		ID:         uuid.New(),
		RFQID:      rfq.ID,
		Maker:      maker,
		Price:      decimal.MustParse(price),
		ValidUntil: validUntil,
		Status:     status,
		CreatedAt:  rfq.CreatedAt,
	}

	if status != domain.QuoteAllocating {
		opened := rfq.CreatedAt
		q.OpenedAt = &opened
	}

	return q
}

func neverOpened(q domain.Quote) domain.Quote {
	q.OpenedAt = nil

	return q
}

func show(legs []domain.Leg) []string {
	out := make([]string, 0, len(legs))

	for _, l := range legs {
		out = append(out, l.ID+": "+l.Sender+" -> "+l.Receiver+" "+l.Amount.String()+" "+l.Instrument)
	}

	return out
}

func legsOf(t *testing.T, rfq domain.RFQ, quote domain.Quote) []domain.Leg {
	t.Helper()

	legs, err := domain.Legs(rfq, quote, operator)
	require.NoError(t, err)

	return legs
}

func sameLegs(expected []domain.Leg) any {
	return mock.MatchedBy(func(legs []domain.Leg) bool { return slices.Equal(show(expected), show(legs)) })
}

func at(expected time.Time) any {
	return mock.MatchedBy(func(v time.Time) bool { return v.Equal(expected) })
}

func quoteIn(id uuid.UUID, status domain.QuoteStatus) any {
	return mock.MatchedBy(func(q domain.Quote) bool { return q.ID == id && q.Status == status })
}

func quoteEvent(q domain.Quote, status domain.QuoteStatus) events.Event {
	return events.Event{
		Type: events.QuoteUpdated,
		Data: dto.QuoteEvent{QuoteID: q.ID, RFQID: q.RFQID, Status: status},
	}
}

func tradeEvent(trade domain.Trade, status domain.TradeStatus) events.Event {
	return events.Event{
		Type: events.TradeUpdated,
		Data: dto.TradeEvent{TradeID: trade.ID, RFQID: trade.RFQID, Status: status},
	}
}

func rfqUpdated(rfq domain.RFQ) events.Event {
	return events.Event{Type: events.RFQUpdated, Data: dto.RFQEvent{RFQID: rfq.ID}}
}

func parties(ps ...domain.Party) []string {
	ids := make([]string, 0, len(ps))

	for _, p := range ps {
		ids = append(ids, p.ID)
	}

	return ids
}

func Test_GetDesk_ReturnsFeeBps(t *testing.T) {
	d := newDeps(t)

	require.Equal(t, dto.GetDeskOutput{FeeBps: 10}, d.useCase(t).GetDesk(t.Context()))
}
