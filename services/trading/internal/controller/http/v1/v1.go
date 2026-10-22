package v1

import (
	"context"

	http_server "github.com/vbloher/canton-private-otc-desk/gen/http/trading_v1/server"
	"github.com/vbloher/canton-private-otc-desk/pkg/principal"
	"github.com/vbloher/canton-private-otc-desk/services/trading/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/trading/internal/usecase"
)

var _ http_server.StrictServerInterface = (*Handlers)(nil)

type Handlers struct {
	usecase *usecase.UseCase
}

func New(uc *usecase.UseCase) *Handlers {
	return &Handlers{
		usecase: uc,
	}
}

// caller is the party the gateway authenticated; every route needs one.
func caller(ctx context.Context) (domain.Party, bool) {
	p, ok := principal.From(ctx)
	if !ok {
		return domain.Party{}, false
	}

	return domain.Party{ID: p.PartyID, Hint: p.Hint}, true
}
