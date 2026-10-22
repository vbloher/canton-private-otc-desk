package v1

import (
	"context"

	pb "github.com/vbloher/canton-private-otc-desk/gen/grpc/ledger_v1"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/dto"
)

func (h Handlers) Allocate(ctx context.Context, i *pb.AllocateInput) (*pb.AllocateOutput, error) {
	output, err := h.usecase.Allocate(ctx, dto.AllocateInput{
		SettlementID: i.GetSettlementId(),
		Deadline:     i.GetDeadline().AsTime(),
		Legs:         toLegs(i.GetLegs()),
		PartyID:      i.GetPartyId(),
		Description:  i.GetDescription(),
	})
	if err != nil {
		return nil, grpcError(ctx, err)
	}

	ids := make([]string, 0, len(output.ActionIDs))
	for _, id := range output.ActionIDs {
		ids = append(ids, id.String())
	}

	return &pb.AllocateOutput{Executed: output.Executed, ActionIds: ids}, nil
}
