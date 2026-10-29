package v1

import (
	"context"
	"errors"

	http_server "github.com/vbloher/canton-private-otc-desk/gen/http/wallet_v1/server"
	"github.com/vbloher/canton-private-otc-desk/pkg/principal"
	"github.com/vbloher/canton-private-otc-desk/pkg/render"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/dto"
)

func (h *Handlers) Faucet(
	ctx context.Context, _ http_server.FaucetRequestObject,
) (http_server.FaucetResponseObject, error) {
	caller, ok := principal.From(ctx)
	if !ok {
		return http_server.Faucet401JSONResponse{UnauthorizedJSONResponse: unauthorized(ctx)}, nil
	}

	output, err := h.usecase.Faucet(ctx, dto.FaucetInput{PartyID: caller.PartyID})

	switch {
	case err == nil:
		return http_server.Faucet202JSONResponse{Actions: toSignActions(output.Actions)}, nil
	case errors.Is(err, domain.ErrFaucetUnavailable):
		return http_server.Faucet404JSONResponse{NotFoundJSONResponse: notFound(ctx, err)}, nil
	case errors.Is(err, domain.ErrFaucetCooldown):
		return http_server.Faucet429JSONResponse{
			Error: render.Error(ctx, err, "request failed").Error(),
		}, nil
	case domain.IsInvalidInput(err) || errors.Is(err, domain.ErrInsufficientFunds):
		return http_server.Faucet400JSONResponse{BadRequestJSONResponse: badRequest(ctx, err)}, nil
	case errors.Is(err, domain.ErrLedger):
		return http_server.Faucet502JSONResponse{BadGatewayJSONResponse: badGateway(ctx, err)}, nil
	default:
		return nil, err
	}
}
