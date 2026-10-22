package v1

import (
	"context"
	"errors"

	http_server "github.com/vbloher/canton-private-otc-desk/gen/http/wallet_v1/server"
	"github.com/vbloher/canton-private-otc-desk/pkg/principal"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/dto"
)

func (h *Handlers) Withdraw(
	ctx context.Context, request http_server.WithdrawRequestObject,
) (http_server.WithdrawResponseObject, error) {
	caller, ok := principal.From(ctx)
	if !ok {
		return http_server.Withdraw401JSONResponse{UnauthorizedJSONResponse: unauthorized(ctx)}, nil
	}

	output, err := h.usecase.Withdraw(ctx, dto.WithdrawInput{
		PartyID:  caller.PartyID,
		Symbol:   request.Body.Symbol,
		Amount:   request.Body.Amount,
		Receiver: request.Body.Receiver,
	})
	if err != nil {
		switch {
		case domain.IsInvalidInput(err):
			return http_server.Withdraw400JSONResponse{BadRequestJSONResponse: badRequest(ctx, err)}, nil
		case isConflict(err):
			return http_server.Withdraw409JSONResponse{ConflictJSONResponse: conflict(ctx, err)}, nil
		case errors.Is(err, domain.ErrLedger):
			return http_server.Withdraw502JSONResponse{BadGatewayJSONResponse: badGateway(ctx, err)}, nil
		default:
			return nil, err
		}
	}

	return http_server.Withdraw202JSONResponse{Actions: toSignActions(output.Actions)}, nil
}
