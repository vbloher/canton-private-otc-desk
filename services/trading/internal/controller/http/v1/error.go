package v1

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"

	nethttpmiddleware "github.com/oapi-codegen/nethttp-middleware"

	http_server "github.com/vbloher/canton-private-otc-desk/gen/http/trading_v1/server"
	"github.com/vbloher/canton-private-otc-desk/pkg/render"
	"github.com/vbloher/canton-private-otc-desk/services/trading/internal/domain"
)

const badGatewayMessage = "bad gateway: the ledger or the identity service failed"

type failure struct {
	status int
	body   http_server.Error
}

func classify(ctx context.Context, err error) failure {
	switch {
	case domain.IsInvalidInput(err):
		return failure{http.StatusBadRequest, http_server.Error{Error: render.Error(ctx, err, "request failed").Error()}}
	case errors.Is(err, domain.ErrNotFound), errors.Is(err, domain.ErrForbidden):
		_ = render.Error(ctx, err, "request failed")

		return failure{http.StatusNotFound, http_server.Error{Error: "request failed: not found"}}
	case errors.Is(err, domain.ErrUpstream):
		_ = render.Error(ctx, err, "bad gateway")

		return failure{http.StatusBadGateway, http_server.Error{Error: badGatewayMessage}}
	}

	code, ok := conflictCode(err)
	if ok {
		return failure{http.StatusConflict, http_server.Error{
			Error: render.Error(ctx, err, "request failed").Error(),
			Code:  &code,
		}}
	}

	return failure{status: http.StatusInternalServerError}
}

func conflictCode(err error) (string, bool) {
	switch {
	case errors.Is(err, domain.ErrRFQClosed):
		return "RFQ_CLOSED", true
	case errors.Is(err, domain.ErrAlreadyQuoted):
		return "ALREADY_QUOTED", true
	case errors.Is(err, domain.ErrQuoteNotOpen):
		return "QUOTE_NOT_OPEN", true
	case errors.Is(err, domain.ErrInsufficientFunds):
		return "INSUFFICIENT_FUNDS", true
	case errors.Is(err, domain.ErrDust):
		return "DUST_TRADE", true
	case errors.Is(err, domain.ErrNotAMaker):
		return "NOT_A_MAKER", true
	default:
		return "", false
	}
}

func unauthorized() http_server.UnauthorizedJSONResponse {
	return http_server.UnauthorizedJSONResponse{Error: "unauthorized"}
}

func RequestErrorHandler(w http.ResponseWriter, r *http.Request, err error) {
	invalidRequest(r.Context(), w, err, http.StatusBadRequest)
}

func ResponseErrorHandler(w http.ResponseWriter, r *http.Request, err error) {
	render.InternalErrorJSON(r.Context(), w, err)
}

func ValidationErrorHandler(
	ctx context.Context, err error, w http.ResponseWriter, _ *http.Request, opts nethttpmiddleware.ErrorHandlerOpts,
) {
	invalidRequest(ctx, w, err, opts.StatusCode)
}

func SchemaErrorMessage(err *openapi3.SchemaError) string {
	if pointer := err.JSONPointer(); len(pointer) > 0 {
		return "/" + strings.Join(pointer, "/") + ": " + err.Reason
	}

	return err.Reason
}

func invalidRequest(ctx context.Context, w http.ResponseWriter, err error, status int) {
	_ = render.Error(ctx, err, "invalid request")

	render.JSON(w, render.Err{Error: "invalid request: " + err.Error()}, status)
}

func (f failure) badRequest() http_server.BadRequestJSONResponse {
	return http_server.BadRequestJSONResponse(f.body)
}

func (f failure) notFound() http_server.NotFoundJSONResponse {
	return http_server.NotFoundJSONResponse(f.body)
}

func (f failure) conflict() http_server.ConflictJSONResponse {
	return http_server.ConflictJSONResponse(f.body)
}

func (f failure) badGateway() http_server.BadGatewayJSONResponse {
	return http_server.BadGatewayJSONResponse(f.body)
}
