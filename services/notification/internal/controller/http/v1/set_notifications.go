package v1

import (
	"context"
	"errors"

	http_server "github.com/vbloher/canton-private-otc-desk/gen/http/notification_v1/server"
	"github.com/vbloher/canton-private-otc-desk/pkg/principal"
	"github.com/vbloher/canton-private-otc-desk/services/notification/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/notification/internal/dto"
)

func (h *Handlers) SetNotifications(
	ctx context.Context, request http_server.SetNotificationsRequestObject,
) (http_server.SetNotificationsResponseObject, error) {
	caller, ok := principal.From(ctx)
	if !ok {
		return http_server.SetNotifications401JSONResponse{UnauthorizedJSONResponse: unauthorized(ctx)}, nil
	}

	output, err := h.usecase.SetPreferences(ctx, dto.SetPreferencesInput{
		PartyID: caller.PartyID,
		News:    request.Body.News,
		Trades:  request.Body.Trades,
	})

	switch {
	case err == nil:
		return http_server.SetNotifications200JSONResponse(toProfile(output)), nil
	case errors.Is(err, domain.ErrEmailNotConfirmed):
		return http_server.SetNotifications409JSONResponse{ConflictJSONResponse: notConfirmed(ctx, err)}, nil
	case errors.Is(err, domain.ErrEmailDisabled):
		return http_server.SetNotifications503JSONResponse{EmailDisabledJSONResponse: emailDisabled(ctx, err)}, nil
	default:
		return nil, err
	}
}
