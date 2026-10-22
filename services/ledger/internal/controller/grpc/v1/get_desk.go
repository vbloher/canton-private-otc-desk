package v1

import (
	"context"

	pb "github.com/vbloher/canton-private-otc-desk/gen/grpc/ledger_v1"
)

func (h Handlers) GetDesk(ctx context.Context, _ *pb.GetDeskInput) (*pb.GetDeskOutput, error) {
	desk := h.usecase.GetDesk(ctx)

	instruments := make([]*pb.Instrument, 0, len(desk.Instruments))

	for _, inst := range desk.Instruments {
		instruments = append(instruments, &pb.Instrument{
			Symbol:   inst.Symbol,
			Name:     inst.Name,
			Decimals: inst.Decimals,
		})
	}

	return &pb.GetDeskOutput{OperatorPartyId: desk.OperatorPartyID, Instruments: instruments}, nil
}
