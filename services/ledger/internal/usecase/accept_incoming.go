package usecase

import (
	"context"
	"fmt"
	"slices"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/dto"
)

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

	intent.Description = fmt.Sprintf("Accept %s %s from %s", transfer.Amount, transfer.Symbol, transfer.SenderHint())
	intent.Counterparty = transfer.Sender

	action, err := u.accept(ctx, intent, transfer)
	if err != nil {
		return output, err
	}

	actions := collect(nil, action)

	err = u.done(ctx, input.PartyID, actions, transfer.Sender)
	if err != nil {
		return output, fmt.Errorf("u.done: %w", err)
	}

	return dto.ActionsOutput{Actions: actions}, nil
}

func (u *UseCase) accept(ctx context.Context, intent domain.Intent, transfer domain.IncomingTransfer,
) (*domain.Action, error) {
	inst, ok := u.config.Instruments.Of(transfer.Admin, transfer.InstrumentID)
	if !ok {
		return nil, domain.Errorf(domain.ErrUnknownInstrument, "%s of %s", transfer.InstrumentID, transfer.Admin)
	}

	cmd, err := u.registry.AcceptTransferCommand(ctx, inst.Registry, transfer.ContractID)
	if err != nil {
		return nil, fmt.Errorf("registry.AcceptTransferCommand: %w", err)
	}

	action, err := u.submit(ctx, intent, cmd, "accept:"+transfer.ContractID)
	if err != nil {
		return nil, fmt.Errorf("u.submit: %w", err)
	}

	return action, nil
}
