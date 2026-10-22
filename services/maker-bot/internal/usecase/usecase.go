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
	Makers   domain.Makers `env:"MAKER_KEYS" envDefault:"demo-key-a:15,demo-key-b:40"`
	Prices   domain.Prices `env:"PRICES_USD" envDefault:"CBTC:100000,CETH:4000,USDCx:1"`
	QuoteTTL time.Duration `env:"QUOTE_TTL"  envDefault:"60s"`
}

// Gateway is the desk's public API, called as the maker the API key belongs to.
type Gateway interface {
	ListIncomingRFQs(ctx context.Context, apiKey string) ([]domain.RFQ, error)
	// SubmitQuote returns domain.ErrConflict when the desk refuses the quote (HTTP 409).
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
