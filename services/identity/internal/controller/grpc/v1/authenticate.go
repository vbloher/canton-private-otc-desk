package v1

import (
	"context"

	pb "github.com/vbloher/canton-private-otc-desk/gen/grpc/identity_v1"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/dto"
)

// Authenticate answers Unauthenticated for a session that is unknown, expired or ended.
func (h Handlers) Authenticate(ctx context.Context, i *pb.AuthenticateInput) (*pb.User, error) {
	output, err := h.usecase.Authenticate(ctx, dto.AuthenticateInput{Token: i.GetSessionToken()})
	if err != nil {
		return nil, grpcError(ctx, err)
	}

	return toUser(output.User), nil
}
