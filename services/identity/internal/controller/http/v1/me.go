package v1

import (
	http_server "github.com/vbloher/canton-private-otc-desk/gen/http/identity_v1/server"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/domain"
)

func toMe(u domain.User) http_server.Me {
	return http_server.Me{
		ID:          u.ID,
		PartyID:     u.PartyID,
		Hint:        u.Hint,
		DisplayName: u.DisplayName,
		Kind:        http_server.MeKind(u.Kind),
		MakerMode:   u.MakerMode,
	}
}
