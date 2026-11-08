package v1

import (
	"context"
	"net/http"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"

	nethttpmiddleware "github.com/oapi-codegen/nethttp-middleware"

	http_server "github.com/vbloher/canton-private-otc-desk/gen/http/notification_v1/server"
	"github.com/vbloher/canton-private-otc-desk/pkg/render"
	"github.com/vbloher/canton-private-otc-desk/services/notification/internal/domain"
)

const (
	messageInvalidEmail    = "Enter a valid email address."
	messageTooManyRequests = "Too many confirmation emails requested. Try again later."
	messageEmailDisabled   = "Email isn't set up on this desk."
	messageNotConfirmed    = "Confirm your email first."
	messageLinkInvalid     = "This link is invalid or has expired."
)

func public(ctx context.Context, err error, message string) string {
	_ = render.Error(ctx, err, "request failed")

	return message
}

func unauthorized(ctx context.Context) http_server.UnauthorizedJSONResponse {
	return http_server.UnauthorizedJSONResponse{Error: public(ctx, domain.ErrUnauthorized, domain.ErrUnauthorized.Error())}
}

func invalidEmail(ctx context.Context, err error) http_server.BadRequestJSONResponse {
	return http_server.BadRequestJSONResponse{Error: public(ctx, err, messageInvalidEmail)}
}

func tooManyRequests(ctx context.Context, err error) http_server.TooManyRequestsJSONResponse {
	return http_server.TooManyRequestsJSONResponse{Error: public(ctx, err, messageTooManyRequests)}
}

func emailDisabled(ctx context.Context, err error) http_server.EmailDisabledJSONResponse {
	return http_server.EmailDisabledJSONResponse{Error: public(ctx, err, messageEmailDisabled)}
}

func notConfirmed(ctx context.Context, err error) http_server.ConflictJSONResponse {
	return http_server.ConflictJSONResponse{Error: public(ctx, err, messageNotConfirmed)}
}

func gone(ctx context.Context, err error) http_server.GoneJSONResponse {
	return http_server.GoneJSONResponse{Error: public(ctx, err, messageLinkInvalid)}
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
