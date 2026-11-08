package v1

import (
	http_server "github.com/vbloher/canton-private-otc-desk/gen/http/notification_v1/server"
	"github.com/vbloher/canton-private-otc-desk/services/notification/internal/dto"
	"github.com/vbloher/canton-private-otc-desk/services/notification/internal/usecase"
)

var _ http_server.StrictServerInterface = (*Handlers)(nil)

type Handlers struct {
	usecase *usecase.UseCase
}

func New(uc *usecase.UseCase) *Handlers {
	return &Handlers{usecase: uc}
}

func toProfile(output dto.ProfileOutput) http_server.Profile {
	profile := http_server.Profile{
		EmailStatus:  http_server.ProfileEmailStatus(output.Profile.Status()),
		News:         output.Profile.News,
		Trades:       output.Profile.Trades,
		EmailEnabled: output.EmailEnabled,
	}

	if output.Profile.Email != "" {
		profile.Email = &output.Profile.Email
	}

	return profile
}
