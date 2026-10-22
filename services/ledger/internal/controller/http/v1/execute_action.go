package v1

import (
	"context"
	"encoding/base64"
	"errors"

	http_server "github.com/vbloher/canton-private-otc-desk/gen/http/wallet_v1/server"
	"github.com/vbloher/canton-private-otc-desk/pkg/principal"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/dto"
)

func (h *Handlers) ExecuteAction(
	ctx context.Context, request http_server.ExecuteActionRequestObject,
) (http_server.ExecuteActionResponseObject, error) {
	caller, ok := principal.From(ctx)
	if !ok {
		return http_server.ExecuteAction401JSONResponse{UnauthorizedJSONResponse: unauthorized(ctx)}, nil
	}

	signature, err := base64.StdEncoding.DecodeString(request.Body.Signature)
	if err != nil {
		err = domain.Errorf(domain.ErrValidation, "signature is not base64: %v", err)

		return http_server.ExecuteAction400JSONResponse{BadRequestJSONResponse: badRequest(ctx, err)}, nil
	}

	err = h.usecase.ExecuteAction(ctx, dto.ExecuteActionInput{
		ID:        request.ID,
		PartyID:   caller.PartyID,
		Signature: signature,
	})
	if err != nil {
		switch {
		// A signature by another key leaves the action pending for its own party.
		case domain.IsInvalidInput(err):
			return http_server.ExecuteAction400JSONResponse{BadRequestJSONResponse: badRequest(ctx, err)}, nil
		case errors.Is(err, domain.ErrNotFound):
			return http_server.ExecuteAction404JSONResponse{NotFoundJSONResponse: notFound(ctx, err)}, nil
		// The signed transaction failed for good (a contract it used is gone): signers treat
		// 404 and 409 as "no longer pending" and drop them silently, so this is a 502.
		case errors.Is(err, domain.ErrContractGone):
			return http_server.ExecuteAction502JSONResponse{BadGatewayJSONResponse: badGateway(ctx, err)}, nil
		case isConflict(err):
			return http_server.ExecuteAction409JSONResponse{ConflictJSONResponse: conflict(ctx, err)}, nil
		case errors.Is(err, domain.ErrLedger):
			return http_server.ExecuteAction502JSONResponse{BadGatewayJSONResponse: badGateway(ctx, err)}, nil
		default:
			return nil, err
		}
	}

	return http_server.ExecuteAction204Response{}, nil
}
