package usecase

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/vbloher/canton-private-otc-desk/pkg/decimal"
	"github.com/vbloher/canton-private-otc-desk/services/maker-bot/internal/domain"
)

//go:generate go tool mockery --config=../../.mockery.yml

type Config struct {
	APIKey   string        `env:"MAKER_API_KEY,notEmpty"`
	Spread   domain.Spread `env:"MAKER_SPREAD_BPS"       envDefault:"15"`
	Prices   domain.Prices `env:"PRICES_USD"             envDefault:"CBTC:100000,USDCx:1,CC:0.15"`
	QuoteTTL time.Duration `env:"QUOTE_TTL"              envDefault:"60s"`
}

type Gateway interface {
	ListIncomingRFQs(ctx context.Context, apiKey string) ([]domain.RFQ, error)
	SubmitQuote(ctx context.Context, apiKey string, rfqID uuid.UUID, price decimal.Decimal, ttl time.Duration) error
}

type UseCase struct {
	config  Config
	gateway Gateway
}

func New(c Config, gateway Gateway) *UseCase {
	return &UseCase{
		config:  c,
		gateway: gateway,
	}
}
