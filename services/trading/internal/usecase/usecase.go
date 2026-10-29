package usecase

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/vbloher/canton-private-otc-desk/pkg/events"
	"github.com/vbloher/canton-private-otc-desk/services/trading/internal/domain"
)

//go:generate go tool mockery --config=../../.mockery.yml

const (
	recentWindow = time.Hour
	listLimit    = 100

	bpsOfTheNotional = 10_000
)

var ErrInvalidConfig = errors.New("invalid config")

type Config struct {
	FeeBps        int64         `env:"FEE_BPS"        envDefault:"10"`
	SettleTimeout time.Duration `env:"SETTLE_TIMEOUT" envDefault:"10m"`
}

func (c Config) Validate() error {
	if c.FeeBps < 0 || c.FeeBps >= bpsOfTheNotional {
		return fmt.Errorf("%w: FEE_BPS must be between 0 and %d, got %d", ErrInvalidConfig, bpsOfTheNotional-1, c.FeeBps)
	}

	if c.SettleTimeout <= 0 {
		return fmt.Errorf("%w: SETTLE_TIMEOUT must be positive, got %s", ErrInvalidConfig, c.SettleTimeout)
	}

	return nil
}

type Transactor interface {
	Wrap(ctx context.Context, fn func(ctx context.Context) error) error
}

//nolint:interfacebloat
type Postgres interface {
	CreateRFQ(ctx context.Context, rfq domain.RFQ) error
	GetRFQForUpdate(ctx context.Context, id uuid.UUID) (domain.RFQ, error)
	UpdateRFQ(ctx context.Context, rfq domain.RFQ) error
	ListRFQs(ctx context.Context, ids []uuid.UUID) ([]domain.RFQ, error)
	ListTakerRFQs(ctx context.Context, taker string, since time.Time) ([]domain.RFQ, error)
	ListMakerRFQs(ctx context.Context, maker string, since time.Time) ([]domain.RFQ, error)
	ListRFQHistory(ctx context.Context, taker string, limit int) ([]domain.RFQ, error)
	ListExpiredRFQs(ctx context.Context, now time.Time) ([]domain.RFQ, error)

	CreateQuote(ctx context.Context, q domain.Quote) error
	GetQuote(ctx context.Context, id uuid.UUID) (domain.Quote, error)
	GetQuoteForUpdate(ctx context.Context, id uuid.UUID) (domain.Quote, error)
	UpdateQuote(ctx context.Context, q domain.Quote) error
	ListQuotes(ctx context.Context, rfqIDs []uuid.UUID) ([]domain.Quote, error)
	ListQuotesByStatus(ctx context.Context, statuses ...domain.QuoteStatus) ([]domain.Quote, error)

	CreateTrade(ctx context.Context, t domain.Trade) error
	UpdateTrade(ctx context.Context, t domain.Trade) error
	ListTrades(ctx context.Context, party string, limit int) ([]domain.Trade, error)
	ListSettlingTrades(ctx context.Context) ([]domain.Trade, error)
}

type Ledger interface {
	Desk(ctx context.Context) (domain.Desk, error)
	Allocate(ctx context.Context, settlementID uuid.UUID, deadline time.Time, legs []domain.Leg,
		party, description string) error
	Settlements(ctx context.Context, queries map[uuid.UUID][]domain.Leg) (map[uuid.UUID]map[string]bool, error)
	Settle(ctx context.Context, settlementID uuid.UUID, legs []domain.Leg) (string, error)
	CancelSettlement(ctx context.Context, settlementID uuid.UUID) error
}

type Identity interface {
	ResolveMakers(ctx context.Context, hints []string) ([]domain.Party, error)
}

type Redis interface {
	GetDesk(ctx context.Context) (domain.Desk, bool)
	SetDesk(ctx context.Context, desk domain.Desk)
}

type Events interface {
	Publish(ctx context.Context, ev events.Event, partyIDs ...string) error
}

type UseCase struct {
	config   Config
	tx       Transactor
	postgres Postgres
	ledger   Ledger
	identity Identity
	redis    Redis
	events   Events
}

func New(
	c Config, tx Transactor, postgres Postgres, ledger Ledger, identity Identity, redis Redis, publisher Events,
) *UseCase {
	return &UseCase{
		config:   c,
		tx:       tx,
		postgres: postgres,
		ledger:   ledger,
		identity: identity,
		redis:    redis,
		events:   publisher,
	}
}

type deal struct {
	rfq   domain.RFQ
	quote domain.Quote
	legs  []domain.Leg
	trade domain.Trade
}
