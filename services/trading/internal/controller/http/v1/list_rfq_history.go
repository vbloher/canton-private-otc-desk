package v1

import (
	"context"

	http_server "github.com/vbloher/canton-private-otc-desk/gen/http/trading_v1/server"
)

func (h *Handlers) ListRfqHistory(
	ctx context.Context, _ http_server.ListRfqHistoryRequestObject,
) (http_server.ListRfqHistoryResponseObject, error) {
	taker, ok := caller(ctx)
	if !ok {
		return http_server.ListRfqHistory401JSONResponse{UnauthorizedJSONResponse: unauthorized()}, nil
	}

	rfqs, err := h.usecase.ListRFQHistory(ctx, taker)
	if err != nil {
		return nil, err
	}

	out := make(http_server.ListRfqHistory200JSONResponse, 0, len(rfqs))
	for _, r := range rfqs {
		out = append(out, toRfq(r))
	}

	return out, nil
}
