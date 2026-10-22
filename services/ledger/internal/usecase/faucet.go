package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/vbloher/canton-private-otc-desk/pkg/decimal"
	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/dto"
)

// Faucet mints test tokens to the caller (sandbox only): the operator, as instrument admin,
// offers each FAUCET_AMOUNTS entry, then the caller accepts every offer.
func (u *UseCase) Faucet(ctx context.Context, input dto.FaucetInput) (dto.ActionsOutput, error) {
	ctx, span := tracer.Start(ctx, "usecase Faucet")
	defer span.End()

	var output dto.ActionsOutput

	rules, err := u.tokenRules(ctx)
	if err != nil {
		return output, fmt.Errorf("u.tokenRules: %w", err)
	}

	offers, err := u.offerMints(ctx, rules, input.PartyID)
	if err != nil {
		return output, err
	}

	var actions []domain.Action

	for _, o := range offers {
		// A mint offer locks no sender holding: there is nothing to disclose but TokenRules.
		cmd, err := u.canton.AcceptTransferCommand(rules, nil, o.contractID)
		if err != nil {
			return output, fmt.Errorf("canton.AcceptTransferCommand: %w", err)
		}

		intent := domain.Intent{
			PartyID:     input.PartyID,
			Purpose:     domain.PurposeTransferAccept,
			Description: fmt.Sprintf("Accept %s %s from the %s", o.amount, o.symbol, domain.FaucetHint),
			Ref:         o.contractID,
		}

		action, err := u.submit(ctx, intent, cmd, "accept:"+o.contractID)
		if err != nil {
			return output, fmt.Errorf("u.submit: %w", err)
		}

		actions = collect(actions, action)
	}

	err = u.done(ctx, input.PartyID, actions)
	if err != nil {
		return output, fmt.Errorf("u.done: %w", err)
	}

	return dto.ActionsOutput{Actions: actions}, nil
}

type mintOffer struct {
	contractID string
	symbol     string
	amount     decimal.Decimal
}

// offerMints submits one mint offer per instrument with a faucet amount, as the operator.
func (u *UseCase) offerMints(ctx context.Context, rules domain.Disclosed, receiver string) ([]mintOffer, error) {
	var offers []mintOffer

	offeredAt := time.Now().Add(-clockSkew)

	for _, inst := range u.config.Instruments {
		amount, ok := u.config.FaucetAmounts[inst.Symbol]
		if !ok {
			continue
		}

		cmd, err := u.canton.OfferMintCommand(rules, domain.OfferMintSpec{
			Admin:        u.operator,
			Receiver:     receiver,
			InstrumentID: inst.Symbol,
			Amount:       amount,
			OfferedAt:    offeredAt,
		})
		if err != nil {
			return nil, fmt.Errorf("canton.OfferMintCommand: %w", err)
		}

		tx, err := u.canton.Submit(ctx, u.operator, "faucet:"+uuid.NewString(), cmd)
		if err != nil {
			return nil, fmt.Errorf("canton.Submit: %w", err)
		}

		if len(tx.Created) == 0 {
			return nil, fmt.Errorf("%w: the %s mint offer created no contract", domain.ErrLedger, inst.Symbol)
		}

		offers = append(offers, mintOffer{contractID: tx.Created[0], symbol: inst.Symbol, amount: amount})
	}

	return offers, nil
}
