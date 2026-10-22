package v1

import (
	"context"

	pb "github.com/vbloher/canton-private-otc-desk/gen/grpc/ledger_v1"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/dto"
)

func (h Handlers) GetSettlements(ctx context.Context, i *pb.GetSettlementsInput) (*pb.GetSettlementsOutput, error) {
	queries := make([]dto.SettlementQuery, 0, len(i.GetSettlements()))

	for _, q := range i.GetSettlements() {
		queries = append(queries, dto.SettlementQuery{SettlementID: q.GetSettlementId(), Legs: toLegs(q.GetLegs())})
	}

	output, err := h.usecase.GetSettlements(ctx, dto.GetSettlementsInput{Settlements: queries})
	if err != nil {
		return nil, grpcError(ctx, err)
	}

	settlements := make([]*pb.SettlementStatus, 0, len(output.Settlements))

	for _, s := range output.Settlements {
		parties := make([]*pb.PartyAllocation, 0, len(s.Parties))
		for _, p := range s.Parties {
			parties = append(parties, &pb.PartyAllocation{PartyId: p.PartyID, Allocated: p.Allocated})
		}

		settlements = append(settlements, &pb.SettlementStatus{SettlementId: s.SettlementID, Parties: parties})
	}

	return &pb.GetSettlementsOutput{Settlements: settlements}, nil
}
