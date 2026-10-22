package v1

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"

	nethttpmiddleware "github.com/oapi-codegen/nethttp-middleware"

	http_server "github.com/vbloher/canton-private-otc-desk/gen/http/wallet_v1/server"
	"github.com/vbloher/canton-private-otc-desk/pkg/render"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/domain"
)

// Conflict codes of the wallet API.
const (
	codeInsufficientFunds = "INSUFFICIENT_FUNDS"
	codeActionNotPending  = "ACTION_NOT_PENDING"
)

var errNoCaller = errors.New("no caller: the gateway forwards it in X-User-Id, X-Party-Id and X-Party-Hint")

func badRequest(ctx context.Context, err error) http_server.BadRequestJSONResponse {
	return http_server.BadRequestJSONResponse{Error: render.Error(ctx, err, "request failed").Error()}
}

func unauthorized(ctx context.Context) http_server.UnauthorizedJSONResponse {
	return http_server.UnauthorizedJSONResponse{Error: render.Error(ctx, errNoCaller, "unauthorized").Error()}
}

func notFound(ctx context.Context, err error) http_server.NotFoundJSONResponse {
	return http_server.NotFoundJSONResponse{Error: render.Error(ctx, err, "request failed").Error()}
}

// conflict answers 409. A contract consumed by another transaction meanwhile (a holding
// another in-flight command used) has no code of its own: the client retries with fresh state.
func conflict(ctx context.Context, err error) http_server.ConflictJSONResponse {
	var code string

	switch {
	case errors.Is(err, domain.ErrInsufficientFunds):
		code = codeInsufficientFunds
	case errors.Is(err, domain.ErrActionNotPending):
		code = codeActionNotPending
	default:
		return http_server.ConflictJSONResponse{
			Error: render.Error(ctx, err, "the ledger changed meanwhile, retry").Error(),
		}
	}

	return http_server.ConflictJSONResponse{Error: render.Error(ctx, err, "request failed").Error(), Code: &code}
}

func badGateway(ctx context.Context, err error) http_server.BadGatewayJSONResponse {
	return http_server.BadGatewayJSONResponse{Error: render.Error(ctx, err, "request failed").Error()}
}

func isConflict(err error) bool {
	return errors.Is(err, domain.ErrInsufficientFunds) ||
		errors.Is(err, domain.ErrActionNotPending) ||
		errors.Is(err, domain.ErrContractGone)
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
