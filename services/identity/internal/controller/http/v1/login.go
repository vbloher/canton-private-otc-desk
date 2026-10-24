package v1

import (
	"context"
	"errors"

	http_server "github.com/vbloher/canton-private-otc-desk/gen/http/identity_v1/server"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/dto"
)

func (h *Handlers) Login(
	ctx context.Context, request http_server.LoginRequestObject,
) (http_server.LoginResponseObject, error) {
	signature, err := decodeBase64("signature", request.Body.Signature)
	if err != nil {
		return http_server.Login400JSONResponse{BadRequestJSONResponse: badRequest(ctx, err)}, nil
	}

	output, err := h.usecase.Login(ctx, dto.LoginInput{
		Challenge:    request.Body.Challenge,
		Signature:    signature,
		CredentialID: request.Body.CredentialID,
	})
	if err != nil {
		switch {
		case domain.IsInvalidInput(err):
			return http_server.Login400JSONResponse{BadRequestJSONResponse: badRequest(ctx, err)}, nil
		case errors.Is(err, domain.ErrUnauthorized):
			return http_server.Login401JSONResponse{UnauthorizedJSONResponse: unauthorized(ctx, err)}, nil
		default:
			return nil, err
		}
	}

	cookie := h.sessionCookie(output.Token, output.TTL)

	return http_server.Login200JSONResponse{
		Body:    toSession(output.User, output.Token, output.ExpiresAt),
		Headers: http_server.Login200ResponseHeaders{SetCookie: &cookie},
	}, nil
}
