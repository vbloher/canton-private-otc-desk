package v1

import (
	"context"
	"net/http"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"

	nethttpmiddleware "github.com/oapi-codegen/nethttp-middleware"

	http_server "github.com/vbloher/canton-private-otc-desk/gen/http/identity_v1/server"
	"github.com/vbloher/canton-private-otc-desk/pkg/render"
)

const ledgerFailedMessage = "ledger request failed"

func badRequest(ctx context.Context, err error) http_server.BadRequestJSONResponse {
	return http_server.BadRequestJSONResponse{Error: render.Error(ctx, err, "request failed").Error()}
}

func unauthorized(ctx context.Context, err error) http_server.UnauthorizedJSONResponse {
	return http_server.UnauthorizedJSONResponse{Error: render.Error(ctx, err, "request failed").Error()}
}

func notFound(ctx context.Context, err error) http_server.NotFoundJSONResponse {
	return http_server.NotFoundJSONResponse{Error: render.Error(ctx, err, "request failed").Error()}
}

func conflict(ctx context.Context, err error) http_server.ConflictJSONResponse {
	return http_server.ConflictJSONResponse{Error: render.Error(ctx, err, "request failed").Error()}
}

func gone(ctx context.Context, err error) http_server.GoneJSONResponse {
	return http_server.GoneJSONResponse{Error: render.Error(ctx, err, "request failed").Error()}
}

// badGateway keeps what the ledger said (addresses, internals) in the log and out of the response.
func badGateway(ctx context.Context, err error) http_server.BadGatewayJSONResponse {
	_ = render.Error(ctx, err, ledgerFailedMessage)

	return http_server.BadGatewayJSONResponse{Error: ledgerFailedMessage}
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
