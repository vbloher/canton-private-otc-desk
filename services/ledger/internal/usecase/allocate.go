package usecase

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/dto"
)

func (u *UseCase) Allocate(ctx context.Context, input dto.AllocateInput) (dto.AllocateOutput, error) {
	ctx, span := tracer.Start(ctx, "usecase Allocate")
	defer span.End()

	var output dto.AllocateOutput

	slots, err := u.slotsToAllocate(input)
	if err != nil {
		return output, err
	}

	queued, err := u.queued(ctx, domain.Intent{
		PartyID: input.PartyID, Purpose: domain.PurposeAllocate, Ref: input.SettlementID,
	})
	if err != nil {
		return output, err
	}

	if len(queued) > 0 {
		return dto.AllocateOutput{ActionIDs: actionIDs(queued)}, nil
	}

	inputs, err := u.allocationInputs(ctx, input.PartyID, slots)
	if err != nil {
		return output, err
	}

	actions, err := u.allocateSlots(ctx, input, slots, inputs)
	if err != nil {
		return output, err
	}

	err = u.done(ctx, input.PartyID, actions)
	if err != nil {
		return output, fmt.Errorf("u.done: %w", err)
	}

	return dto.AllocateOutput{Executed: u.hosted[input.PartyID], ActionIDs: actionIDs(actions)}, nil
}

func (u *UseCase) allocateSlots(ctx context.Context, input dto.AllocateInput, slots []domain.Slot, inputs [][]string,
) ([]domain.Action, error) {
	requestedAt := time.Now().Add(-clockSkew)
	actions := make([]domain.Action, 0, len(slots))

	for i, slot := range slots {
		cmd, err := u.registry.AllocateCommand(ctx, domain.AllocateSpec{
			Operator:     u.operator,
			SettlementID: input.SettlementID,
			Slot:         slot,
			Deadline:     input.Deadline,
			RequestedAt:  requestedAt,
			Inputs:       inputs[i],
		})
		if err != nil {
			return nil, fmt.Errorf("registry.AllocateCommand: %w", err)
		}

		intent := domain.Intent{
			PartyID:     input.PartyID,
			Purpose:     domain.PurposeAllocate,
			Description: slotDescription(slot, input.Description),
			Ref:         input.SettlementID,
		}
		commandID := "allocate:" + input.SettlementID + ":" + input.PartyID + ":" + slot.InstrumentID

		action, err := u.submit(ctx, intent, cmd, commandID)
		if err != nil {
			return nil, fmt.Errorf("u.submit: %w", err)
		}

		actions = collect(actions, action)
	}

	return actions, nil
}

func actionIDs(actions []domain.Action) []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(actions))

	for _, action := range actions {
		ids = append(ids, action.ID)
	}

	return ids
}

func (u *UseCase) slotsToAllocate(input dto.AllocateInput) ([]domain.Slot, error) {
	if input.SettlementID == "" || input.PartyID == "" {
		return nil, domain.Errorf(domain.ErrValidation, "settlement id and party are required")
	}

	if !input.Deadline.After(time.Now()) {
		return nil, domain.Errorf(domain.ErrValidation, "the settlement deadline %s has passed",
			input.Deadline.UTC().Format(time.RFC3339))
	}

	legs, err := u.legs(input.Legs)
	if err != nil {
		return nil, err
	}

	slots := domain.SlotsOf(domain.Plan(legs), input.PartyID)
	if len(slots) == 0 {
		return nil, domain.Errorf(domain.ErrValidation, "%s has no leg in settlement %s", input.PartyID,
			input.SettlementID)
	}

	return slots, nil
}

func (u *UseCase) allocationInputs(ctx context.Context, party string, slots []domain.Slot) ([][]string, error) {
	inputs := make([][]string, len(slots))

	if !slices.ContainsFunc(slots, func(s domain.Slot) bool { return s.Send.IsPositive() }) {
		return inputs, nil
	}

	holdings, err := u.canton.Holdings(ctx, party)
	if err != nil {
		return nil, fmt.Errorf("canton.Holdings: %w", err)
	}

	asOf := locksAsOf()

	for i, slot := range slots {
		if !slot.Send.IsPositive() {
			continue
		}

		inputs[i], err = domain.SelectHoldings(holdings, party, slot.Admin, slot.InstrumentID, slot.Send, asOf)
		if err != nil {
			return nil, fmt.Errorf("domain.SelectHoldings: %w", err)
		}
	}

	return inputs, nil
}

func slotDescription(slot domain.Slot, about string) string {
	text := "Allocate receipt of " + slot.Symbol
	if slot.Send.IsPositive() {
		text = "Allocate " + slot.Send.String() + " " + slot.Symbol
	}

	if about != "" {
		text += " for " + about
	}

	return text
}
