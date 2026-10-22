package usecase

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/vbloher/canton-private-otc-desk/pkg/decimal"
	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/dto"
)

// transferWindow is how long a receiver has to accept a withdrawal before it lapses.
const transferWindow = 24 * time.Hour

// Withdraw offers holdings to any Canton party; the receiver accepts the offer. Every
// withdrawal is a new intent; only a double-submitted one (same amount, instrument and
// receiver, still waiting for the signature) gets the pending action back.
func (u *UseCase) Withdraw(ctx context.Context, input dto.WithdrawInput) (dto.ActionsOutput, error) {
	ctx, span := tracer.Start(ctx, "usecase Withdraw")
	defer span.End()

	var output dto.ActionsOutput

	inst, amount, err := u.validateWithdraw(input)
	if err != nil {
		return output, err
	}

	intent := domain.Intent{
		PartyID:      input.PartyID,
		Purpose:      domain.PurposeTransferOut,
		Description:  fmt.Sprintf("Withdraw %s %s to %s", amount, inst.Symbol, domain.Hint(input.Receiver)),
		Ref:          input.Receiver,
		Counterparty: input.Receiver, // it receives the offer to accept
	}

	queued, err := u.queued(ctx, intent)
	if err != nil {
		return output, err
	}

	if len(queued) > 0 {
		return dto.ActionsOutput{Actions: queued}, nil
	}

	cmd, err := u.transferCommand(ctx, input, inst.Symbol, amount)
	if err != nil {
		return output, err
	}

	action, err := u.submit(ctx, intent, cmd, "withdraw:"+uuid.NewString())
	if err != nil {
		return output, fmt.Errorf("u.submit: %w", err)
	}

	actions := collect(nil, action)

	err = u.done(ctx, input.PartyID, actions, input.Receiver)
	if err != nil {
		return output, fmt.Errorf("u.done: %w", err)
	}

	return dto.ActionsOutput{Actions: actions}, nil
}

// transferCommand offers amount of the instrument from the sender's own unlocked holdings.
func (u *UseCase) transferCommand(ctx context.Context, input dto.WithdrawInput, symbol string, amount decimal.Decimal,
) (domain.Command, error) {
	holdings, err := u.canton.Holdings(ctx, input.PartyID)
	if err != nil {
		return domain.Command{}, fmt.Errorf("canton.Holdings: %w", err)
	}

	inputs, err := domain.SelectHoldings(holdings, input.PartyID, u.operator, symbol, amount, locksAsOf())
	if err != nil {
		return domain.Command{}, fmt.Errorf("domain.SelectHoldings: %w", err)
	}

	rules, err := u.tokenRules(ctx)
	if err != nil {
		return domain.Command{}, fmt.Errorf("u.tokenRules: %w", err)
	}

	now := time.Now()

	cmd, err := u.canton.TransferCommand(rules, domain.TransferSpec{
		Admin:         u.operator,
		Sender:        input.PartyID,
		Receiver:      input.Receiver,
		InstrumentID:  symbol,
		Amount:        amount,
		Inputs:        inputs,
		RequestedAt:   now.Add(-clockSkew),
		ExecuteBefore: now.Add(transferWindow),
	})
	if err != nil {
		return domain.Command{}, fmt.Errorf("canton.TransferCommand: %w", err)
	}

	return cmd, nil
}

func (u *UseCase) validateWithdraw(input dto.WithdrawInput) (domain.Instrument, decimal.Decimal, error) {
	inst, ok := u.config.Instruments.Find(input.Symbol)
	if !ok {
		return domain.Instrument{}, decimal.Zero, domain.Errorf(domain.ErrUnknownInstrument, "%q", input.Symbol)
	}

	amount, err := decimal.Parse(input.Amount)
	if err != nil || !amount.IsPositive() {
		return domain.Instrument{}, decimal.Zero, domain.Errorf(domain.ErrValidation,
			"amount %q is not a positive decimal", input.Amount)
	}

	if !strings.Contains(input.Receiver, "::") {
		return domain.Instrument{}, decimal.Zero, domain.Errorf(domain.ErrValidation,
			"receiver %q must be a full party id", input.Receiver)
	}

	if input.Receiver == input.PartyID {
		return domain.Instrument{}, decimal.Zero, domain.Errorf(domain.ErrValidation, "cannot withdraw to yourself")
	}

	return inst, amount, nil
}
