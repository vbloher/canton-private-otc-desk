package v1

import (
	"context"
	"errors"

	http_server "github.com/vbloher/canton-private-otc-desk/gen/http/identity_v1/server"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/dto"
)

func (h *Handlers) RegisterComplete(
	ctx context.Context, request http_server.RegisterCompleteRequestObject,
) (http_server.RegisterCompleteResponseObject, error) {
	signature, err := decodeBase64("signature", request.Body.Signature)
	if err != nil {
		return http_server.RegisterComplete400JSONResponse{BadRequestJSONResponse: badRequest(ctx, err)}, nil
	}

	output, err := h.usecase.CompleteRegistration(ctx, dto.CompleteRegistrationInput{
		RegistrationID: request.Body.RegistrationID,
		Signature:      signature,
		CredentialID:   request.Body.CredentialID,
		EncryptedBlob:  request.Body.EncryptedBlob,
	})
	if err != nil {
		switch {
		case domain.IsInvalidInput(err):
			return http_server.RegisterComplete400JSONResponse{BadRequestJSONResponse: badRequest(ctx, err)}, nil
		case errors.Is(err, domain.ErrCredentialTaken):
			return http_server.RegisterComplete409JSONResponse{ConflictJSONResponse: conflict(ctx, err)}, nil
		case errors.Is(err, domain.ErrRegistrationExpired):
			return http_server.RegisterComplete410JSONResponse{GoneJSONResponse: gone(ctx, err)}, nil
		case errors.Is(err, domain.ErrLedger):
			return http_server.RegisterComplete502JSONResponse{BadGatewayJSONResponse: badGateway(ctx, err)}, nil
		default:
			return nil, err
		}
	}

	cookie := h.sessionCookie(output.Token, output.TTL)

	return http_server.RegisterComplete200JSONResponse{
		Body:    toSession(output.User, output.Token, output.ExpiresAt),
		Headers: http_server.RegisterComplete200ResponseHeaders{SetCookie: &cookie},
	}, nil
}
