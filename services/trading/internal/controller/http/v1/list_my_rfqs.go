package v1

import (
	"context"

	http_server "github.com/vbloher/canton-private-otc-desk/gen/http/trading_v1/server"
)

func (h *Handlers) ListMyRfqs(
	ctx context.Context, _ http_server.ListMyRfqsRequestObject,
) (http_server.ListMyRfqsResponseObject, error) {
	taker, ok := caller(ctx)
	if !ok {
		return http_server.ListMyRfqs401JSONResponse{UnauthorizedJSONResponse: unauthorized()}, nil
	}

	rfqs, err := h.usecase.ListMyRFQs(ctx, taker)
	if err != nil {
		return nil, err
	}

	out := make(http_server.ListMyRfqs200JSONResponse, 0, len(rfqs))

	for _, r := range rfqs {
		quotes := make([]http_server.Quote, 0, len(r.Quotes))
		for _, q := range r.Quotes {
			quotes = append(quotes, toQuote(q))
		}

		out = append(out, http_server.RfqWithQuotes{Rfq: toRfq(r.RFQ), Quotes: quotes})
	}

	return out, nil
}
