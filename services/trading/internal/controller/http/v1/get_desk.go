package v1

import (
	"context"

	http_server "github.com/vbloher/canton-private-otc-desk/gen/http/trading_v1/server"
)

func (h *Handlers) GetDesk(
	ctx context.Context, _ http_server.GetDeskRequestObject,
) (http_server.GetDeskResponseObject, error) {
	if _, ok := caller(ctx); !ok {
		return http_server.GetDesk401JSONResponse{UnauthorizedJSONResponse: unauthorized()}, nil
	}

	output := h.usecase.GetDesk(ctx)

	return http_server.GetDesk200JSONResponse{FeeBps: int(output.FeeBps)}, nil
}
