package v1

import (
	"context"
	"errors"

	http_server "github.com/vbloher/canton-private-otc-desk/gen/http/identity_v1/server"
	"github.com/vbloher/canton-private-otc-desk/pkg/principal"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/dto"
)

func (h *Handlers) GetMe(
	ctx context.Context, _ http_server.GetMeRequestObject,
) (http_server.GetMeResponseObject, error) {
	caller, ok := principal.From(ctx)
	if !ok {
		return http_server.GetMe401JSONResponse{UnauthorizedJSONResponse: unauthorized(ctx, domain.ErrUnauthorized)}, nil
	}

	output, err := h.usecase.GetMe(ctx, dto.GetMeInput{UserID: caller.UserID})
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return http_server.GetMe401JSONResponse{UnauthorizedJSONResponse: unauthorized(ctx, domain.ErrUnauthorized)}, nil
		}

		return nil, err
	}

	return http_server.GetMe200JSONResponse(toMe(output.User)), nil
}
