package v1

import (
	pb "github.com/vbloher/canton-private-otc-desk/gen/grpc/ledger_v1"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/dto"
)

func toLegs(legs []*pb.TransferLeg) []dto.TransferLeg {
	out := make([]dto.TransferLeg, 0, len(legs))

	for _, l := range legs {
		out = append(out, dto.TransferLeg{
			ID:         l.GetId(),
			Sender:     l.GetSender(),
			Receiver:   l.GetReceiver(),
			Instrument: l.GetInstrument(),
			Amount:     l.GetAmount(),
		})
	}

	return out
}
