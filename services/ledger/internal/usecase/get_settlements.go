package usecase

import (
	"context"
	"fmt"
	"maps"
	"slices"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/dto"
)

func (u *UseCase) GetSettlements(ctx context.Context, input dto.GetSettlementsInput) (dto.GetSettlementsOutput, error) {
	ctx, span := tracer.Start(ctx, "usecase GetSettlements")
	defer span.End()

	var output dto.GetSettlementsOutput

	if len(input.Settlements) == 0 {
		return output, nil
	}

	plans := make([][]domain.Slot, len(input.Settlements))

	for i, q := range input.Settlements {
		legs, err := u.legs(q.Legs)
		if err != nil {
			return output, fmt.Errorf("settlement %s: %w", q.SettlementID, err)
		}

		plans[i] = domain.Plan(legs)
	}

	allocations, err := u.canton.Allocations(ctx, u.operator)
	if err != nil {
		return output, fmt.Errorf("canton.Allocations: %w", err)
	}

	output.Settlements = make([]dto.SettlementStatus, 0, len(input.Settlements))

	for i, q := range input.Settlements {
		status := domain.PartyStatus(domain.MatchSlots(allocations, q.SettlementID, plans[i]))
		parties := make([]dto.PartyAllocation, 0, len(status))

		for _, party := range slices.Sorted(maps.Keys(status)) {
			parties = append(parties, dto.PartyAllocation{PartyID: party, Allocated: status[party]})
		}

		output.Settlements = append(output.Settlements, dto.SettlementStatus{SettlementID: q.SettlementID, Parties: parties})
	}

	return output, nil
}
