package v1

import (
	"context"
	"errors"

	http_server "github.com/vbloher/canton-private-otc-desk/gen/http/wallet_v1/server"
	"github.com/vbloher/canton-private-otc-desk/pkg/principal"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/dto"
)

func (h *Handlers) GetBalances(
	ctx context.Context, _ http_server.GetBalancesRequestObject,
) (http_server.GetBalancesResponseObject, error) {
	caller, ok := principal.From(ctx)
	if !ok {
		return http_server.GetBalances401JSONResponse{UnauthorizedJSONResponse: unauthorized(ctx)}, nil
	}

	output, err := h.usecase.GetBalances(ctx, dto.GetBalancesInput{PartyID: caller.PartyID})
	if err != nil {
		if errors.Is(err, domain.ErrLedger) {
			return http_server.GetBalances502JSONResponse{BadGatewayJSONResponse: badGateway(ctx, err)}, nil
		}

		return nil, err
	}

	balances := make(http_server.GetBalances200JSONResponse, 0, len(output.Balances))

	for _, b := range output.Balances {
		balances = append(balances, http_server.Balance{
			Symbol:    b.Symbol,
			Available: b.Available.String(),
			Locked:    b.Locked.String(),
		})
	}

	return balances, nil
}
