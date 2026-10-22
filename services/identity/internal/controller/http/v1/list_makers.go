package v1

import (
	"context"

	http_server "github.com/vbloher/canton-private-otc-desk/gen/http/identity_v1/server"
	"github.com/vbloher/canton-private-otc-desk/pkg/principal"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/dto"
)

func (h *Handlers) ListMakers(
	ctx context.Context, _ http_server.ListMakersRequestObject,
) (http_server.ListMakersResponseObject, error) {
	caller, ok := principal.From(ctx)
	if !ok {
		return http_server.ListMakers401JSONResponse{
			UnauthorizedJSONResponse: unauthorized(ctx, domain.ErrUnauthorized),
		}, nil
	}

	output, err := h.usecase.ListMakers(ctx, dto.ListMakersInput{CallerID: caller.UserID})
	if err != nil {
		return nil, err
	}

	makers := make([]http_server.Maker, 0, len(output.Makers))
	for _, maker := range output.Makers {
		makers = append(makers, http_server.Maker{Hint: maker.Hint, DisplayName: maker.DisplayName})
	}

	return http_server.ListMakers200JSONResponse(makers), nil
}
