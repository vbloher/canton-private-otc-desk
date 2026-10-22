package v1

import (
	"context"
	"errors"

	http_server "github.com/vbloher/canton-private-otc-desk/gen/http/identity_v1/server"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/dto"
)

func (h *Handlers) RegisterStart(
	ctx context.Context, request http_server.RegisterStartRequestObject,
) (http_server.RegisterStartResponseObject, error) {
	publicKey, err := decodeBase64("publicKey", request.Body.PublicKey)
	if err != nil {
		return http_server.RegisterStart400JSONResponse{BadRequestJSONResponse: badRequest(ctx, err)}, nil
	}

	output, err := h.usecase.StartRegistration(ctx, dto.StartRegistrationInput{
		DisplayName: request.Body.DisplayName,
		PublicKey:   publicKey,
	})
	if err != nil {
		switch {
		case domain.IsInvalidInput(err):
			return http_server.RegisterStart400JSONResponse{BadRequestJSONResponse: badRequest(ctx, err)}, nil
		case errors.Is(err, domain.ErrLedger):
			return http_server.RegisterStart502JSONResponse{BadGatewayJSONResponse: badGateway(ctx, err)}, nil
		default:
			return nil, err
		}
	}

	return http_server.RegisterStart200JSONResponse{
		RegistrationID: output.RegistrationID,
		PartyID:        output.PartyID,
		MultiHash:      encodeBase64(output.MultiHash),
	}, nil
}
