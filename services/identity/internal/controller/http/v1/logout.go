package v1

import (
	"context"

	http_server "github.com/vbloher/canton-private-otc-desk/gen/http/identity_v1/server"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/dto"
)

// Logout answers 204 and clears the cookie whether or not the cookie held a live session.
func (h *Handlers) Logout(
	ctx context.Context, request http_server.LogoutRequestObject,
) (http_server.LogoutResponseObject, error) {
	var token string
	if request.Params.OtcSession != nil {
		token = *request.Params.OtcSession
	}

	err := h.usecase.Logout(ctx, dto.LogoutInput{Token: token})
	if err != nil {
		return nil, err
	}

	cookie := h.clearedCookie()

	return http_server.Logout204Response{Headers: http_server.Logout204ResponseHeaders{SetCookie: &cookie}}, nil
}
