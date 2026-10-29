package usecase

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/dto"
)

func (u *UseCase) CancelSettlement(
	ctx context.Context, input dto.CancelSettlementInput,
) (dto.CancelSettlementOutput, error) {
	ctx, span := tracer.Start(ctx, "usecase CancelSettlement")
	defer span.End()

	var output dto.CancelSettlementOutput

	if input.SettlementID == "" {
		return output, domain.Errorf(domain.ErrValidation, "settlement id is required")
	}

	err := u.expireQueued(ctx, input.SettlementID)
	if err != nil {
		return output, err
	}

	allocations, err := u.canton.Allocations(ctx, u.operator)
	if err != nil {
		return output, fmt.Errorf("canton.Allocations: %w", err)
	}

	allocations = slices.DeleteFunc(allocations, func(a domain.Allocation) bool {
		return a.SettlementID != input.SettlementID
	})
	if len(allocations) == 0 {
		return output, nil
	}

	output.Cancelled, err = u.cancelAll(ctx, allocations)

	return output, err
}

func (u *UseCase) cancelAll(ctx context.Context, allocations []domain.Allocation) (int, error) {
	var (
		cancelled   int
		errs        []error
		authorizers []string
	)

	for _, a := range allocations {
		err := u.cancel(ctx, a)
		if err != nil {
			errs = append(errs, err)

			continue
		}

		cancelled++

		if !slices.Contains(authorizers, a.Authorizer) {
			authorizers = append(authorizers, a.Authorizer)
		}
	}

	if len(authorizers) > 0 {
		u.publish(ctx, walletUpdated(), authorizers...)
	}

	return cancelled, errors.Join(errs...)
}

func (u *UseCase) expireQueued(ctx context.Context, settlementID string) error {
	expired, err := u.postgres.ExpireActionsByRef(ctx, settlementID)
	if err != nil {
		return fmt.Errorf("postgres.ExpireActionsByRef: %w", err)
	}

	for _, action := range expired {
		u.publish(ctx, signCompleted(action.ID), action.PartyID)
	}

	return nil
}

func (u *UseCase) cancel(ctx context.Context, allocation domain.Allocation) error {
	registry, ok := u.config.Instruments.Registry(allocation.Admin)
	if !ok {
		return domain.Errorf(domain.ErrUnknownInstrument, "no instrument of %s", allocation.Admin)
	}

	cmd, err := u.registry.CancelCommand(ctx, registry, allocation.ContractID, u.operator)
	if errors.Is(err, domain.ErrContractGone) {
		return nil
	}

	if err != nil {
		return fmt.Errorf("registry.CancelCommand: %w", err)
	}

	_, err = u.canton.Submit(ctx, u.operator, "cancel:"+allocation.ContractID, cmd)
	if err != nil && !errors.Is(err, domain.ErrContractGone) && !errors.Is(err, domain.ErrDuplicateCommand) {
		return fmt.Errorf("canton.Submit cancel %s: %w", allocation.ContractID, err)
	}

	return nil
}
