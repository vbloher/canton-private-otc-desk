package v1

import (
	"context"
	"errors"

	http_server "github.com/vbloher/canton-private-otc-desk/gen/http/identity_v1/server"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/dto"
)

func (h *Handlers) RegisterParty(
	ctx context.Context, request http_server.RegisterPartyRequestObject,
) (http_server.RegisterPartyResponseObject, error) {
	signature, err := decodeBase64("signature", request.Body.Signature)
	if err != nil {
		return http_server.RegisterParty400JSONResponse{BadRequestJSONResponse: badRequest(ctx, err)}, nil
	}

	output, err := h.usecase.RegisterParty(ctx, dto.RegisterPartyInput{
		RegistrationID: request.Body.RegistrationID,
		Signature:      signature,
	})
	if err != nil {
		switch {
		case domain.IsInvalidInput(err):
			return http_server.RegisterParty400JSONResponse{BadRequestJSONResponse: badRequest(ctx, err)}, nil
		case errors.Is(err, domain.ErrRegistrationExpired):
			return http_server.RegisterParty410JSONResponse{GoneJSONResponse: gone(ctx, err)}, nil
		case errors.Is(err, domain.ErrOnboardingPaused):
			return http_server.RegisterParty503JSONResponse{OnboardingPausedJSONResponse: onboardingPaused(ctx, err)}, nil
		case errors.Is(err, domain.ErrLedger):
			return http_server.RegisterParty502JSONResponse{BadGatewayJSONResponse: badGateway(ctx, err)}, nil
		default:
			return nil, err
		}
	}

	return http_server.RegisterParty200JSONResponse{PartyID: output.PartyID}, nil
}
