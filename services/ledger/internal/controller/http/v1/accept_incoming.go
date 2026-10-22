package v1

import (
	"context"
	"errors"

	http_server "github.com/vbloher/canton-private-otc-desk/gen/http/wallet_v1/server"
	"github.com/vbloher/canton-private-otc-desk/pkg/principal"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/dto"
)

func (h *Handlers) AcceptIncoming(
	ctx context.Context, request http_server.AcceptIncomingRequestObject,
) (http_server.AcceptIncomingResponseObject, error) {
	caller, ok := principal.From(ctx)
	if !ok {
		return http_server.AcceptIncoming401JSONResponse{UnauthorizedJSONResponse: unauthorized(ctx)}, nil
	}

	output, err := h.usecase.AcceptIncoming(ctx, dto.AcceptIncomingInput{
		PartyID:    caller.PartyID,
		ContractID: request.ContractID,
	})
	if err != nil {
		switch {
		// A transfer consumed meanwhile (accepted, withdrawn, its lock gone) is no longer pending.
		case errors.Is(err, domain.ErrNotFound), errors.Is(err, domain.ErrContractGone):
			return http_server.AcceptIncoming404JSONResponse{NotFoundJSONResponse: notFound(ctx, err)}, nil
		case errors.Is(err, domain.ErrLedger):
			return http_server.AcceptIncoming502JSONResponse{BadGatewayJSONResponse: badGateway(ctx, err)}, nil
		default:
			return nil, err
		}
	}

	return http_server.AcceptIncoming202JSONResponse{Actions: toSignActions(output.Actions)}, nil
}
