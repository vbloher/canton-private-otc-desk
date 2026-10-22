package v1

import (
	"context"

	http_server "github.com/vbloher/canton-private-otc-desk/gen/http/trading_v1/server"
)

func (h *Handlers) ListIncomingRfqs(
	ctx context.Context, _ http_server.ListIncomingRfqsRequestObject,
) (http_server.ListIncomingRfqsResponseObject, error) {
	maker, ok := caller(ctx)
	if !ok {
		return http_server.ListIncomingRfqs401JSONResponse{UnauthorizedJSONResponse: unauthorized()}, nil
	}

	rfqs, err := h.usecase.ListIncomingRFQs(ctx, maker)
	if err != nil {
		return nil, err
	}

	out := make(http_server.ListIncomingRfqs200JSONResponse, 0, len(rfqs))

	for _, r := range rfqs {
		incoming := http_server.IncomingRfq{Rfq: toRfq(r.RFQ)}

		if r.MyQuote != nil {
			mine := toQuote(*r.MyQuote)
			incoming.MyQuote = &mine
		}

		out = append(out, incoming)
	}

	return out, nil
}
