package usecase

import (
	"context"
	"errors"
	"fmt"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/dto"
)

func (u *UseCase) Settle(ctx context.Context, input dto.SettleInput) (dto.SettleOutput, error) {
	ctx, span := tracer.Start(ctx, "usecase Settle")
	defer span.End()

	var output dto.SettleOutput

	if input.SettlementID == "" {
		return output, domain.Errorf(domain.ErrValidation, "settlement id is required")
	}

	legs, err := u.legs(input.Legs)
	if err != nil {
		return output, err
	}

	plan := domain.Plan(legs)

	matches, err := u.planAllocations(ctx, input.SettlementID, plan)
	if err != nil {
		return output, err
	}

	cmd, err := u.registry.SettleCommand(ctx, domain.SettleSpec{
		Operator:     u.operator,
		SettlementID: input.SettlementID,
		Batches:      domain.Batches(legs, matches),
	})
	if err != nil {
		return output, fmt.Errorf("registry.SettleCommand: %w", err)
	}

	tx, err := u.canton.Submit(ctx, u.operator, "settle:"+input.SettlementID, cmd)
	if err != nil && !errors.Is(err, domain.ErrDuplicateCommand) {
		return output, fmt.Errorf("canton.Submit: %w", err)
	}

	u.publish(ctx, walletUpdated(), domain.Parties(plan)...)

	return dto.SettleOutput{UpdateID: tx.UpdateID}, nil
}

func (u *UseCase) planAllocations(ctx context.Context, settlementID string, plan []domain.Slot,
) ([]domain.SlotMatch, error) {
	allocations, err := u.canton.Allocations(ctx, u.operator)
	if err != nil {
		return nil, fmt.Errorf("canton.Allocations: %w", err)
	}

	matches := domain.MatchSlots(allocations, settlementID, plan)

	for _, m := range matches {
		if m.ContractID == "" {
			return nil, domain.Errorf(domain.ErrNotFound, "settlement %s has no allocation for %s", settlementID, m.Key())
		}
	}

	return matches, nil
}
