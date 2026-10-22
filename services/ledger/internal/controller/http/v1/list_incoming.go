package v1

import (
	"context"
	"errors"

	http_server "github.com/vbloher/canton-private-otc-desk/gen/http/wallet_v1/server"
	"github.com/vbloher/canton-private-otc-desk/pkg/principal"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/dto"
)

func (h *Handlers) ListIncoming(
	ctx context.Context, _ http_server.ListIncomingRequestObject,
) (http_server.ListIncomingResponseObject, error) {
	caller, ok := principal.From(ctx)
	if !ok {
		return http_server.ListIncoming401JSONResponse{UnauthorizedJSONResponse: unauthorized(ctx)}, nil
	}

	output, err := h.usecase.ListIncoming(ctx, dto.ListIncomingInput{PartyID: caller.PartyID})
	if err != nil {
		if errors.Is(err, domain.ErrLedger) {
			return http_server.ListIncoming502JSONResponse{BadGatewayJSONResponse: badGateway(ctx, err)}, nil
		}

		return nil, err
	}

	transfers := make(http_server.ListIncoming200JSONResponse, 0, len(output.Transfers))

	for _, t := range output.Transfers {
		transfers = append(transfers, http_server.IncomingTransfer{
			ContractID: t.ContractID,
			Sender:     t.SenderHint(),
			Symbol:     t.InstrumentID,
			Amount:     t.Amount.String(),
		})
	}

	return transfers, nil
}
