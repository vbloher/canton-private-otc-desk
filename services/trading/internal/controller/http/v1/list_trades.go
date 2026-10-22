package v1

import (
	"context"

	http_server "github.com/vbloher/canton-private-otc-desk/gen/http/trading_v1/server"
)

func (h *Handlers) ListTrades(
	ctx context.Context, _ http_server.ListTradesRequestObject,
) (http_server.ListTradesResponseObject, error) {
	party, ok := caller(ctx)
	if !ok {
		return http_server.ListTrades401JSONResponse{UnauthorizedJSONResponse: unauthorized()}, nil
	}

	trades, err := h.usecase.ListTrades(ctx, party)
	if err != nil {
		return nil, err
	}

	out := make(http_server.ListTrades200JSONResponse, 0, len(trades))
	for _, t := range trades {
		out = append(out, toTrade(t))
	}

	return out, nil
}
