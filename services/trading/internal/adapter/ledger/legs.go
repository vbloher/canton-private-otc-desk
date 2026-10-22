package ledger

import (
	"github.com/vbloher/canton-private-otc-desk/gen/grpc/ledger_v1"
	"github.com/vbloher/canton-private-otc-desk/services/trading/internal/domain"
)

func toLegs(legs []domain.Leg) []*ledger_v1.TransferLeg {
	out := make([]*ledger_v1.TransferLeg, 0, len(legs))

	for _, l := range legs {
		out = append(out, &ledger_v1.TransferLeg{
			Id:         l.ID,
			Sender:     l.Sender,
			Receiver:   l.Receiver,
			Instrument: l.Instrument,
			Amount:     l.Amount.String(),
		})
	}

	return out
}
