package v1

import (
	"context"
	"net/http"

	http_server "github.com/vbloher/canton-private-otc-desk/gen/http/trading_v1/server"
	"github.com/vbloher/canton-private-otc-desk/services/trading/internal/dto"
)

func (h *Handlers) SubmitQuote(
	ctx context.Context, request http_server.SubmitQuoteRequestObject,
) (http_server.SubmitQuoteResponseObject, error) {
	maker, ok := caller(ctx)
	if !ok {
		return http_server.SubmitQuote401JSONResponse{UnauthorizedJSONResponse: unauthorized()}, nil
	}

	quote, err := h.usecase.SubmitQuote(ctx, dto.SubmitQuoteInput{
		Maker:      maker,
		RFQID:      request.ID,
		Price:      request.Body.Price,
		TTLSeconds: request.Body.TTLSeconds,
	})
	if err != nil {
		f := classify(ctx, err)

		switch f.status {
		case http.StatusBadRequest:
			return http_server.SubmitQuote400JSONResponse{BadRequestJSONResponse: f.badRequest()}, nil
		case http.StatusNotFound:
			return http_server.SubmitQuote404JSONResponse{NotFoundJSONResponse: f.notFound()}, nil
		case http.StatusConflict:
			return http_server.SubmitQuote409JSONResponse{ConflictJSONResponse: f.conflict()}, nil
		case http.StatusBadGateway:
			return http_server.SubmitQuote502JSONResponse{BadGatewayJSONResponse: f.badGateway()}, nil
		default:
			return nil, err
		}
	}

	return http_server.SubmitQuote201JSONResponse(toQuote(quote)), nil
}
