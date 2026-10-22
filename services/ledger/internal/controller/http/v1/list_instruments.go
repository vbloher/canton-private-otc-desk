package v1

import (
	"context"

	http_server "github.com/vbloher/canton-private-otc-desk/gen/http/wallet_v1/server"
	"github.com/vbloher/canton-private-otc-desk/pkg/principal"
)

func (h *Handlers) ListInstruments(
	ctx context.Context, _ http_server.ListInstrumentsRequestObject,
) (http_server.ListInstrumentsResponseObject, error) {
	if _, ok := principal.From(ctx); !ok {
		return http_server.ListInstruments401JSONResponse{UnauthorizedJSONResponse: unauthorized(ctx)}, nil
	}

	desk := h.usecase.GetDesk(ctx)

	instruments := make(http_server.ListInstruments200JSONResponse, 0, len(desk.Instruments))

	for _, inst := range desk.Instruments {
		instruments = append(instruments, http_server.Instrument{
			Symbol:   inst.Symbol,
			Name:     inst.Name,
			Decimals: int(inst.Decimals),
		})
	}

	return instruments, nil
}
