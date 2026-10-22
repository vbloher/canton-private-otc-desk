package usecase

import (
	"context"
	"fmt"
	"slices"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/dto"
)

// AcceptIncoming accepts a pending transfer to the caller (a deposit). An external party
// whose accept still waits for its signature gets that action back.
func (u *UseCase) AcceptIncoming(ctx context.Context, input dto.AcceptIncomingInput) (dto.ActionsOutput, error) {
	ctx, span := tracer.Start(ctx, "usecase AcceptIncoming")
	defer span.End()

	var output dto.ActionsOutput

	intent := domain.Intent{PartyID: input.PartyID, Purpose: domain.PurposeTransferAccept, Ref: input.ContractID}

	queued, err := u.queued(ctx, intent)
	if err != nil {
		return output, err
	}

	if len(queued) > 0 {
		return dto.ActionsOutput{Actions: queued}, nil
	}

	transfers, err := u.incoming(ctx, input.PartyID)
	if err != nil {
		return output, err
	}

	i := slices.IndexFunc(transfers, func(t domain.IncomingTransfer) bool { return t.ContractID == input.ContractID })
	if i < 0 {
		return output, domain.Errorf(domain.ErrNotFound, "no pending transfer %s", input.ContractID)
	}

	transfer := transfers[i]

	rules, err := u.tokenRules(ctx)
	if err != nil {
		return output, fmt.Errorf("u.tokenRules: %w", err)
	}

	inputs, err := u.lockedInputs(ctx, transfer)
	if err != nil {
		return output, err
	}

	cmd, err := u.canton.AcceptTransferCommand(rules, inputs, transfer.ContractID)
	if err != nil {
		return output, fmt.Errorf("canton.AcceptTransferCommand: %w", err)
	}

	intent.Description = fmt.Sprintf("Accept %s %s from %s", transfer.Amount, transfer.InstrumentID,
		transfer.SenderHint())
	intent.Counterparty = transfer.Sender // its locked holding goes away; none for a mint

	action, err := u.submit(ctx, intent, cmd, "accept:"+transfer.ContractID)
	if err != nil {
		return output, fmt.Errorf("u.submit: %w", err)
	}

	actions := collect(nil, action)

	err = u.done(ctx, input.PartyID, actions, transfer.Sender)
	if err != nil {
		return output, fmt.Errorf("u.done: %w", err)
	}

	return dto.ActionsOutput{Actions: actions}, nil
}

// lockedInputs are the sender's holdings the transfer locked, as their admin discloses them.
func (u *UseCase) lockedInputs(ctx context.Context, transfer domain.IncomingTransfer) ([]domain.Disclosed, error) {
	if len(transfer.Inputs) == 0 {
		return nil, nil
	}

	tokens, err := u.canton.Tokens(ctx, u.operator)
	if err != nil {
		return nil, fmt.Errorf("canton.Tokens: %w", err)
	}

	inputs := slices.DeleteFunc(tokens, func(d domain.Disclosed) bool {
		return !slices.Contains(transfer.Inputs, d.ContractID)
	})

	return inputs, nil
}
