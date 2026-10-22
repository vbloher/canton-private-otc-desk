package v1

import (
	"context"

	pb "github.com/vbloher/canton-private-otc-desk/gen/grpc/ledger_v1"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/dto"
)

func (h Handlers) CancelSettlement(
	ctx context.Context, i *pb.CancelSettlementInput,
) (*pb.CancelSettlementOutput, error) {
	output, err := h.usecase.CancelSettlement(ctx, dto.CancelSettlementInput{SettlementID: i.GetSettlementId()})
	if err != nil {
		return nil, grpcError(ctx, err)
	}

	return &pb.CancelSettlementOutput{Cancelled: int32(output.Cancelled)}, nil //nolint:gosec // a handful per settlement
}
