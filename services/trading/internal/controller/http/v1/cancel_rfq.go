package v1

import (
	"context"
	"net/http"

	http_server "github.com/vbloher/canton-private-otc-desk/gen/http/trading_v1/server"
	"github.com/vbloher/canton-private-otc-desk/services/trading/internal/dto"
)

func (h *Handlers) CancelRfq(
	ctx context.Context, request http_server.CancelRfqRequestObject,
) (http_server.CancelRfqResponseObject, error) {
	taker, ok := caller(ctx)
	if !ok {
		return http_server.CancelRfq401JSONResponse{UnauthorizedJSONResponse: unauthorized()}, nil
	}

	err := h.usecase.CancelRFQ(ctx, dto.CancelRFQInput{Taker: taker, RFQID: request.ID})
	if err != nil {
		f := classify(ctx, err)

		switch f.status {
		case http.StatusNotFound:
			return http_server.CancelRfq404JSONResponse{NotFoundJSONResponse: f.notFound()}, nil
		case http.StatusConflict:
			return http_server.CancelRfq409JSONResponse{ConflictJSONResponse: f.conflict()}, nil
		default:
			return nil, err
		}
	}

	return http_server.CancelRfq204Response{}, nil
}
