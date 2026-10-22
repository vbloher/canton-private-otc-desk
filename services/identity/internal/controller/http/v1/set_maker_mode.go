package v1

import (
	"context"
	"errors"

	http_server "github.com/vbloher/canton-private-otc-desk/gen/http/identity_v1/server"
	"github.com/vbloher/canton-private-otc-desk/pkg/principal"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/dto"
)

func (h *Handlers) SetMakerMode(
	ctx context.Context, request http_server.SetMakerModeRequestObject,
) (http_server.SetMakerModeResponseObject, error) {
	caller, ok := principal.From(ctx)
	if !ok {
		return http_server.SetMakerMode401JSONResponse{
			UnauthorizedJSONResponse: unauthorized(ctx, domain.ErrUnauthorized),
		}, nil
	}

	output, err := h.usecase.SetMakerMode(ctx, dto.SetMakerModeInput{
		UserID:  caller.UserID,
		Enabled: request.Body.Enabled,
	})
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return http_server.SetMakerMode401JSONResponse{
				UnauthorizedJSONResponse: unauthorized(ctx, domain.ErrUnauthorized),
			}, nil
		}

		return nil, err
	}

	return http_server.SetMakerMode200JSONResponse(toMe(output.User)), nil
}
