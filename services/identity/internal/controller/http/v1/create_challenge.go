package v1

import (
	"context"
	"errors"

	http_server "github.com/vbloher/canton-private-otc-desk/gen/http/identity_v1/server"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/dto"
)

func (h *Handlers) CreateChallenge(
	ctx context.Context, request http_server.CreateChallengeRequestObject,
) (http_server.CreateChallengeResponseObject, error) {
	output, err := h.usecase.CreateChallenge(ctx, dto.CreateChallengeInput{
		CredentialID: request.Body.CredentialID,
	})
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrNotFound):
			return http_server.CreateChallenge404JSONResponse{NotFoundJSONResponse: notFound(ctx, err)}, nil
		case domain.IsInvalidInput(err):
			return http_server.CreateChallenge400JSONResponse{BadRequestJSONResponse: badRequest(ctx, err)}, nil
		default:
			return nil, err
		}
	}

	return http_server.CreateChallenge200JSONResponse{
		Challenge:     output.Challenge,
		EncryptedBlob: output.EncryptedBlob,
		PublicKey:     encodeBase64(output.PublicKey),
	}, nil
}
