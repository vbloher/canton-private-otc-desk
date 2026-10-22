package usecase

import (
	"context"
	"errors"
	"fmt"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/dto"
)

// Settle settles every leg atomically in one transaction, as the operator, from the
// plan-matched allocations. The command id derives from the settlement: a re-submitted
// settle answered with DUPLICATE_COMMAND has already landed.
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

	plan := domain.Plan(legs, u.operator)

	cids, err := u.planAllocations(ctx, input.SettlementID, plan)
	if err != nil {
		return output, err
	}

	rules, err := u.tokenRules(ctx)
	if err != nil {
		return output, fmt.Errorf("u.tokenRules: %w", err)
	}

	tokens, err := u.canton.Tokens(ctx, u.operator)
	if err != nil {
		return output, fmt.Errorf("canton.Tokens: %w", err)
	}

	cmd, err := u.canton.SettleCommand(rules, tokens, domain.SettleSpec{
		Operator:       u.operator,
		SettlementID:   input.SettlementID,
		Legs:           legs,
		AllocationCIDs: cids,
	})
	if err != nil {
		return output, fmt.Errorf("canton.SettleCommand: %w", err)
	}

	tx, err := u.canton.Submit(ctx, u.operator, "settle:"+input.SettlementID, cmd)
	if err != nil && !errors.Is(err, domain.ErrDuplicateCommand) {
		return output, fmt.Errorf("canton.Submit: %w", err)
	}

	u.publish(ctx, walletUpdated(), domain.Parties(plan)...)

	return dto.SettleOutput{UpdateID: tx.UpdateID}, nil
}

// planAllocations are the allocations of every slot, in plan order: a stray allocation
// carrying the same settlement id never joins the batch.
func (u *UseCase) planAllocations(ctx context.Context, settlementID string, plan []domain.Slot) ([]string, error) {
	allocations, err := u.canton.Allocations(ctx, u.operator)
	if err != nil {
		return nil, fmt.Errorf("canton.Allocations: %w", err)
	}

	cids := make([]string, 0, len(plan))

	for _, m := range domain.MatchSlots(allocations, settlementID, plan) {
		if m.ContractID == "" {
			return nil, domain.Errorf(domain.ErrNotFound, "settlement %s has no allocation for %s", settlementID, m.Key())
		}

		cids = append(cids, m.ContractID)
	}

	return cids, nil
}
