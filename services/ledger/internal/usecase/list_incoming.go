package usecase

import (
	"context"
	"fmt"
	"slices"

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

// incoming are the pending transfers to the party in the desk's instruments: those the
// operator administers.
func (u *UseCase) incoming(ctx context.Context, party string) ([]domain.IncomingTransfer, error) {
	transfers, err := u.canton.TransferInstructions(ctx, party)
	if err != nil {
		return nil, fmt.Errorf("canton.TransferInstructions: %w", err)
	}

	return slices.DeleteFunc(transfers, func(t domain.IncomingTransfer) bool { return t.Admin != u.operator }), nil
}
