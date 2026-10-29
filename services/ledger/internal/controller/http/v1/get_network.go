package v1

import (
	"context"

	http_server "github.com/vbloher/canton-private-otc-desk/gen/http/wallet_v1/server"
	"github.com/vbloher/canton-private-otc-desk/pkg/principal"
)

func (h *Handlers) GetNetwork(
	ctx context.Context, _ http_server.GetNetworkRequestObject,
) (http_server.GetNetworkResponseObject, error) {
	if _, ok := principal.From(ctx); !ok {
		return http_server.GetNetwork401JSONResponse{UnauthorizedJSONResponse: unauthorized(ctx)}, nil
	}

	output := h.usecase.GetNetwork(ctx)

	faucet := make([]http_server.Drip, 0, len(output.Faucet))
	for _, drip := range output.Faucet {
		faucet = append(faucet, http_server.Drip{Symbol: drip.Symbol, Amount: drip.Amount.String()})
	}

	return http_server.GetNetwork200JSONResponse{
		Name:   http_server.NetworkName(output.Network),
		Faucet: faucet,
	}, nil
}
