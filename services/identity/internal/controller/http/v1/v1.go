package v1

import (
	http_server "github.com/vbloher/canton-private-otc-desk/gen/http/identity_v1/server"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/usecase"
)

var _ http_server.StrictServerInterface = (*Handlers)(nil)

type Handlers struct {
	usecase      *usecase.UseCase
	cookieSecure bool
}

func New(uc *usecase.UseCase, cookieSecure bool) *Handlers {
	return &Handlers{
		usecase:      uc,
		cookieSecure: cookieSecure,
	}
}
