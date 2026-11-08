package v1

import (
	"context"

	http_server "github.com/vbloher/canton-private-otc-desk/gen/http/notification_v1/server"
	"github.com/vbloher/canton-private-otc-desk/pkg/principal"
	"github.com/vbloher/canton-private-otc-desk/services/notification/internal/dto"
)

func (h *Handlers) GetProfile(
	ctx context.Context, _ http_server.GetProfileRequestObject,
) (http_server.GetProfileResponseObject, error) {
	caller, ok := principal.From(ctx)
	if !ok {
		return http_server.GetProfile401JSONResponse{UnauthorizedJSONResponse: unauthorized(ctx)}, nil
	}

	output, err := h.usecase.GetProfile(ctx, dto.GetProfileInput{PartyID: caller.PartyID})
	if err != nil {
		return nil, err
	}

	return http_server.GetProfile200JSONResponse(toProfile(output)), nil
}
