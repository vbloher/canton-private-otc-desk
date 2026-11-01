package usecase

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"

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

	var drips []domain.Instrument

	for _, inst := range u.config.Instruments {
		if _, ok := u.config.FaucetAmounts[inst.Symbol]; ok {
			drips = append(drips, inst)
		}
	}

	created := make([][]string, len(drips))
	errs := make([]error, len(drips))

	var wg sync.WaitGroup

	for i, inst := range drips {
		wg.Go(func() {
			created[i], errs[i] = u.dripOne(ctx, holdings, inst, receiver)
		})
	}

	wg.Wait()

	var (
		all    []string
		failed []error
	)

	for i, inst := range drips {
		if errs[i] != nil {
			log.Warn().Err(errs[i]).Str("symbol", inst.Symbol).Str("receiver", receiver).Msg("ledger: faucet drip failed")
			failed = append(failed, fmt.Errorf("%s: %w", inst.Symbol, errs[i]))

			continue
		}

		all = append(all, created[i]...)
	}

	if len(failed) == len(drips) {
		return nil, fmt.Errorf("the faucet sent nothing: %w", errors.Join(failed...))
	}

	return all, nil
}

func (u *UseCase) dripOne(ctx context.Context, holdings []domain.Holding, inst domain.Instrument, receiver string,
) ([]string, error) {
	cmd, err := u.transferCommand(ctx, holdings, inst, u.faucet, receiver, u.config.FaucetAmounts[inst.Symbol])
	if err != nil {
		return nil, err
	}

	tx, err := u.canton.Submit(ctx, u.faucet, "faucet:"+uuid.NewString(), cmd)
	if err != nil {
		return nil, fmt.Errorf("canton.Submit: %w", err)
	}

	return tx.Created, nil
}

func (u *UseCase) acceptDrip(ctx context.Context, receiver string, created []string) ([]domain.Action, error) {
	transfers, err := u.incoming(ctx, receiver)
	if err != nil {
		return nil, err
	}

	transfers = slices.DeleteFunc(transfers, func(t domain.IncomingTransfer) bool {
		return t.Sender != u.faucet || !slices.Contains(created, t.ContractID)
	})

	accepted := make([]*domain.Action, len(transfers))
	errs := make([]error, len(transfers))

	var wg sync.WaitGroup

	for i, t := range transfers {
		intent := domain.Intent{
			PartyID:     receiver,
			Purpose:     domain.PurposeTransferAccept,
			Description: fmt.Sprintf("Accept %s %s from the %s", t.Amount, t.Symbol, domain.FaucetHint),
			Ref:         t.ContractID,
		}

		wg.Go(func() {
			accepted[i], errs[i] = u.accept(ctx, intent, t)
		})
	}

	wg.Wait()

	err = errors.Join(errs...)
	if err != nil {
		return nil, err
	}

	var actions []domain.Action

	for _, action := range accepted {
		actions = collect(actions, action)
	}

	return actions, nil
}
