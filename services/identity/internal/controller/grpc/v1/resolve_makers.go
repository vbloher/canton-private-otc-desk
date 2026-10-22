package v1

import (
	"context"

	pb "github.com/vbloher/canton-private-otc-desk/gen/grpc/identity_v1"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/dto"
)

func (h Handlers) ResolveMakers(ctx context.Context, i *pb.ResolveMakersInput) (*pb.ResolveMakersOutput, error) {
	output, err := h.usecase.ResolveMakers(ctx, dto.ResolveMakersInput{Hints: i.GetHints()})
	if err != nil {
		return nil, grpcError(ctx, err)
	}

	makers := make([]*pb.User, 0, len(output.Makers))
	for _, maker := range output.Makers {
		makers = append(makers, toUser(maker))
	}

	return &pb.ResolveMakersOutput{Makers: makers}, nil
}
