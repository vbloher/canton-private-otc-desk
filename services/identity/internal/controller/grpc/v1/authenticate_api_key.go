package v1

import (
	"context"

	pb "github.com/vbloher/canton-private-otc-desk/gen/grpc/identity_v1"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/dto"
)

// AuthenticateAPIKey answers Unauthenticated for an API key nobody holds.
func (h Handlers) AuthenticateAPIKey(ctx context.Context, i *pb.AuthenticateAPIKeyInput) (*pb.User, error) {
	output, err := h.usecase.AuthenticateAPIKey(ctx, dto.AuthenticateAPIKeyInput{APIKey: i.GetApiKey()})
	if err != nil {
		return nil, grpcError(ctx, err)
	}

	return toUser(output.User), nil
}
