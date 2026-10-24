package v1

import (
	"context"

	http_server "github.com/vbloher/canton-private-otc-desk/gen/http/identity_v1/server"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/dto"
)

// Logout ends every session the request names: the one in the cookie and the one in "Authorization:
// Bearer <token>" (a frontend on another origin has no cookie). They can differ, and the gateway
// prefers the bearer token over the cookie, so neither may be left alive; when they are the same
// session it ends once. A missing or unusable header is no token. It answers 204 and clears the
// cookie whether or not the tokens held live sessions.
func (h *Handlers) Logout(
	ctx context.Context, request http_server.LogoutRequestObject,
) (http_server.LogoutResponseObject, error) {
	var cookie string
	if request.Params.OtcSession != nil {
		cookie = *request.Params.OtcSession
	}

	err := h.usecase.Logout(ctx, dto.LogoutInput{Tokens: []string{cookie, bearerFrom(ctx)}})
	if err != nil {
		return nil, err
	}

	cleared := h.clearedCookie()

	return http_server.Logout204Response{Headers: http_server.Logout204ResponseHeaders{SetCookie: &cleared}}, nil
}
