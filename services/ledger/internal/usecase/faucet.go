package usecase

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/dto"
)

func (u *UseCase) Faucet(ctx context.Context, input dto.FaucetInput) (dto.ActionsOutput, error) {
	ctx, span := tracer.Start(ctx, "usecase Faucet")
	defer span.End()

	var output dto.ActionsOutput

	if u.faucet == "" {
		return output, domain.ErrFaucetUnavailable
	}

	if input.PartyID == u.faucet {
		return output, domain.Errorf(domain.ErrValidation, "the faucet cannot pay itself")
	}

	u.faucetMu.Lock()
	defer u.faucetMu.Unlock()

	claimed, err := u.redis.ClaimFaucet(ctx, input.PartyID, u.config.FaucetCooldown)
	if err != nil {
		return output, fmt.Errorf("redis.ClaimFaucet: %w", err)
	}

	if !claimed {
		return output, domain.Errorf(domain.ErrFaucetCooldown, "one drip per %s", u.config.FaucetCooldown)
	}

	created, err := u.drip(ctx, input.PartyID)
	if err != nil {
		u.redis.ReleaseFaucet(ctx, input.PartyID)

		return output, err
	}

	actions, err := u.acceptDrip(ctx, input.PartyID, created)
	if err != nil {
		return output, err
	}

	err = u.done(ctx, input.PartyID, actions, u.faucet)
	if err != nil {
		return output, fmt.Errorf("u.done: %w", err)
	}

	return dto.ActionsOutput{Actions: actions}, nil
}

func (u *UseCase) drip(ctx context.Context, receiver string) ([]string, error) {
	holdings, err := u.canton.Holdings(ctx, u.faucet)
	if err != nil {
		return nil, fmt.Errorf("canton.Holdings: %w", err)
	}

	var (
		created []string
		errs    []error
		sent    int
	)

	for _, inst := range u.config.Instruments {
		amount, ok := u.config.FaucetAmounts[inst.Symbol]
		if !ok {
			continue
		}

		cmd, err := u.transferCommand(ctx, holdings, inst, u.faucet, receiver, amount)
		if err == nil {
			var tx domain.Transaction

			tx, err = u.canton.Submit(ctx, u.faucet, "faucet:"+uuid.NewString(), cmd)
			created = append(created, tx.Created...)
		}

		if err != nil {
			log.Warn().Err(err).Str("symbol", inst.Symbol).Str("receiver", receiver).Msg("ledger: faucet drip failed")
			errs = append(errs, fmt.Errorf("%s: %w", inst.Symbol, err))

			continue
		}

		sent++
	}

	if sent == 0 {
		return nil, fmt.Errorf("the faucet sent nothing: %w", errors.Join(errs...))
	}

	return created, nil
}

func (u *UseCase) acceptDrip(ctx context.Context, receiver string, created []string) ([]domain.Action, error) {
	transfers, err := u.incoming(ctx, receiver)
	if err != nil {
		return nil, err
	}

	var actions []domain.Action

	for _, t := range transfers {
		if t.Sender != u.faucet || !slices.Contains(created, t.ContractID) {
			continue
		}

		intent := domain.Intent{
			PartyID:     receiver,
			Purpose:     domain.PurposeTransferAccept,
			Description: fmt.Sprintf("Accept %s %s from the %s", t.Amount, t.Symbol, domain.FaucetHint),
			Ref:         t.ContractID,
		}

		action, err := u.accept(ctx, intent, t)
		if err != nil {
			return nil, err
		}

		actions = collect(actions, action)
	}

	return actions, nil
}
