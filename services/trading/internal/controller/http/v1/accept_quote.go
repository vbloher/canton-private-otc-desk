package v1

import (
	"context"
	"net/http"

	http_server "github.com/vbloher/canton-private-otc-desk/gen/http/trading_v1/server"
	"github.com/vbloher/canton-private-otc-desk/services/trading/internal/dto"
)

func (h *Handlers) AcceptQuote(
	ctx context.Context, request http_server.AcceptQuoteRequestObject,
) (http_server.AcceptQuoteResponseObject, error) {
	taker, ok := caller(ctx)
	if !ok {
		return http_server.AcceptQuote401JSONResponse{UnauthorizedJSONResponse: unauthorized()}, nil
	}

	trade, err := h.usecase.AcceptQuote(ctx, dto.AcceptQuoteInput{Taker: taker, QuoteID: request.ID})
	if err != nil {
		f := classify(ctx, err)

		switch f.status {
		case http.StatusNotFound:
			return http_server.AcceptQuote404JSONResponse{NotFoundJSONResponse: f.notFound()}, nil
		case http.StatusConflict:
			return http_server.AcceptQuote409JSONResponse{ConflictJSONResponse: f.conflict()}, nil
		case http.StatusBadGateway:
			return http_server.AcceptQuote502JSONResponse{BadGatewayJSONResponse: f.badGateway()}, nil
		default:
			return nil, err
		}
	}

	return http_server.AcceptQuote202JSONResponse(toTrade(trade)), nil
}
