package usecase

import (
	"context"
	"fmt"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/dto"
)

func (u *UseCase) ListIncoming(ctx context.Context, input dto.ListIncomingInput) (dto.ListIncomingOutput, error) {
	ctx, span := tracer.Start(ctx, "usecase ListIncoming")
	defer span.End()

	var output dto.ListIncomingOutput

	transfers, err := u.incoming(ctx, input.PartyID)
	if err != nil {
		return output, err
	}

	return dto.ListIncomingOutput{Transfers: transfers}, nil
}

func (u *UseCase) incoming(ctx context.Context, party string) ([]domain.IncomingTransfer, error) {
	transfers, err := u.canton.TransferInstructions(ctx, party)
	if err != nil {
		return nil, fmt.Errorf("canton.TransferInstructions: %w", err)
	}

	known := make([]domain.IncomingTransfer, 0, len(transfers))

	for _, t := range transfers {
		inst, ok := u.config.Instruments.Of(t.Admin, t.InstrumentID)
		if !ok {
			continue
		}

		t.Symbol = inst.Symbol
		known = append(known, t)
	}

	return known, nil
}
