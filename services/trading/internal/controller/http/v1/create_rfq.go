package v1

import (
	"context"
	"net/http"

	http_server "github.com/vbloher/canton-private-otc-desk/gen/http/trading_v1/server"
	"github.com/vbloher/canton-private-otc-desk/services/trading/internal/dto"
)

func (h *Handlers) CreateRfq(
	ctx context.Context, request http_server.CreateRfqRequestObject,
) (http_server.CreateRfqResponseObject, error) {
	taker, ok := caller(ctx)
	if !ok {
		return http_server.CreateRfq401JSONResponse{UnauthorizedJSONResponse: unauthorized()}, nil
	}

	rfq, err := h.usecase.CreateRFQ(ctx, dto.CreateRFQInput{
		Taker:      taker,
		Direction:  string(request.Body.Direction),
		Qty:        request.Body.Qty,
		Base:       request.Body.Base,
		Quote:      request.Body.Quote,
		Makers:     request.Body.Makers,
		TTLSeconds: request.Body.TTLSeconds,
	})
	if err != nil {
		f := classify(ctx, err)

		switch f.status {
		case http.StatusBadRequest:
			return http_server.CreateRfq400JSONResponse{BadRequestJSONResponse: f.badRequest()}, nil
		case http.StatusConflict:
			return http_server.CreateRfq409JSONResponse{ConflictJSONResponse: f.conflict()}, nil
		case http.StatusBadGateway:
			return http_server.CreateRfq502JSONResponse{BadGatewayJSONResponse: f.badGateway()}, nil
		default:
			return nil, err
		}
	}

	return http_server.CreateRfq201JSONResponse(toRfq(rfq)), nil
}
