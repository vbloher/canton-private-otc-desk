package v1

import (
	"context"
	"errors"

	http_server "github.com/vbloher/canton-private-otc-desk/gen/http/notification_v1/server"
	"github.com/vbloher/canton-private-otc-desk/pkg/principal"
	"github.com/vbloher/canton-private-otc-desk/services/notification/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/notification/internal/dto"
)

func (h *Handlers) SetEmail(
	ctx context.Context, request http_server.SetEmailRequestObject,
) (http_server.SetEmailResponseObject, error) {
	caller, ok := principal.From(ctx)
	if !ok {
		return http_server.SetEmail401JSONResponse{UnauthorizedJSONResponse: unauthorized(ctx)}, nil
	}

	output, err := h.usecase.SetEmail(ctx, dto.SetEmailInput{PartyID: caller.PartyID, Email: request.Body.Email})

	switch {
	case err == nil:
		return http_server.SetEmail200JSONResponse(toProfile(output)), nil
	case errors.Is(err, domain.ErrValidation):
		return http_server.SetEmail400JSONResponse{BadRequestJSONResponse: invalidEmail(ctx, err)}, nil
	case errors.Is(err, domain.ErrTooManyRequests):
		return http_server.SetEmail429JSONResponse{TooManyRequestsJSONResponse: tooManyRequests(ctx, err)}, nil
	case errors.Is(err, domain.ErrEmailDisabled):
		return http_server.SetEmail503JSONResponse{EmailDisabledJSONResponse: emailDisabled(ctx, err)}, nil
	default:
		return nil, err
	}
}
