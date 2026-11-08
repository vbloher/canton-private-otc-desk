package v1

import (
	"context"
	"errors"

	http_server "github.com/vbloher/canton-private-otc-desk/gen/http/notification_v1/server"
	"github.com/vbloher/canton-private-otc-desk/services/notification/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/notification/internal/dto"
)

func (h *Handlers) ConfirmEmail(
	ctx context.Context, request http_server.ConfirmEmailRequestObject,
) (http_server.ConfirmEmailResponseObject, error) {
	err := h.usecase.ConfirmEmail(ctx, dto.ConfirmEmailInput{Token: request.Body.Token})

	switch {
	case err == nil:
		return http_server.ConfirmEmail204Response{}, nil
	case errors.Is(err, domain.ErrConfirmationInvalid):
		return http_server.ConfirmEmail410JSONResponse{GoneJSONResponse: gone(ctx, err)}, nil
	default:
		return nil, err
	}
}
