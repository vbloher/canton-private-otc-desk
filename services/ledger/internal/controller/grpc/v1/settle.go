package v1

import (
	"context"

	pb "github.com/vbloher/canton-private-otc-desk/gen/grpc/ledger_v1"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/dto"
)

func (h Handlers) Settle(ctx context.Context, i *pb.SettleInput) (*pb.SettleOutput, error) {
	output, err := h.usecase.Settle(ctx, dto.SettleInput{
		SettlementID: i.GetSettlementId(),
		Legs:         toLegs(i.GetLegs()),
	})
	if err != nil {
		return nil, grpcError(ctx, err)
	}

	return &pb.SettleOutput{UpdateId: output.UpdateID}, nil
}
