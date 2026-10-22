package v1

import (
	"context"

	pb "github.com/vbloher/canton-private-otc-desk/gen/grpc/ledger_v1"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/dto"
)

func (h Handlers) ResolveParty(ctx context.Context, i *pb.ResolvePartyInput) (*pb.ResolvePartyOutput, error) {
	output, err := h.usecase.ResolveParty(ctx, dto.ResolvePartyInput{Hint: i.GetHint()})
	if err != nil {
		return nil, grpcError(ctx, err)
	}

	return &pb.ResolvePartyOutput{PartyId: output.PartyID}, nil
}
