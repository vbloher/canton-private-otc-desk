package ledger

import (
	"context"
	"fmt"

	"github.com/vbloher/canton-private-otc-desk/gen/grpc/ledger_v1"
	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/trading/internal/domain"
)

func (l *Ledger) Desk(ctx context.Context) (domain.Desk, error) {
	ctx, span := tracer.Start(ctx, "adapter ledger Desk")
	defer span.End()

	out, err := l.client.GetDesk(ctx, &ledger_v1.GetDeskInput{})
	if err != nil {
		return domain.Desk{}, wrap("GetDesk", err)
	}

	if out.GetOperatorPartyId() == "" {
		return domain.Desk{}, fmt.Errorf("%w: ledger GetDesk: no operator party", domain.ErrUpstream)
	}

	instruments := make([]string, 0, len(out.GetInstruments()))
	for _, i := range out.GetInstruments() {
		instruments = append(instruments, i.GetSymbol())
	}

	return domain.Desk{Operator: out.GetOperatorPartyId(), Instruments: instruments}, nil
}
