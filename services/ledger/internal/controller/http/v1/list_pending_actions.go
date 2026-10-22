package v1

import (
	"context"

	http_server "github.com/vbloher/canton-private-otc-desk/gen/http/wallet_v1/server"
	"github.com/vbloher/canton-private-otc-desk/pkg/principal"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/dto"
)

func (h *Handlers) ListPendingActions(
	ctx context.Context, _ http_server.ListPendingActionsRequestObject,
) (http_server.ListPendingActionsResponseObject, error) {
	caller, ok := principal.From(ctx)
	if !ok {
		return http_server.ListPendingActions401JSONResponse{UnauthorizedJSONResponse: unauthorized(ctx)}, nil
	}

	output, err := h.usecase.ListPendingActions(ctx, dto.ListPendingActionsInput{PartyID: caller.PartyID})
	if err != nil {
		return nil, err
	}

	return http_server.ListPendingActions200JSONResponse(toSignActions(output.Actions)), nil
}
