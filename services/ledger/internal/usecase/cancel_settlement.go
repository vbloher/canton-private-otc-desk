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

// CancelSettlement cancels, as the operator, every allocation of a dead settlement that is
// still active; an allocation already gone counts as cancelled. First it expires every
// allocate action of the settlement still waiting for a signature: one signed after the
// cancel would leave a committed allocation of a dead settlement behind. An execution in
// flight holds its row until the ledger has answered, so its allocation is on the ledger,
// and cancelled here, by the time the actions are expired.
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

	rules, err := u.tokenRules(ctx)
	if err != nil {
		return output, fmt.Errorf("u.tokenRules: %w", err)
	}

	tokens, err := u.canton.Tokens(ctx, u.operator)
	if err != nil {
		return output, fmt.Errorf("canton.Tokens: %w", err)
	}

	output.Cancelled, err = u.cancelAll(ctx, rules, tokens, allocations)

	return output, err
}

// cancelAll cancels every allocation it can and tells their authorizers their holdings are
// free again; the rest fail together.
func (u *UseCase) cancelAll(ctx context.Context, rules domain.Disclosed, tokens []domain.Disclosed,
	allocations []domain.Allocation,
) (int, error) {
	var (
		cancelled   int
		errs        []error
		authorizers []string
	)

	for _, a := range allocations {
		err := u.cancel(ctx, rules, tokens, a.ContractID)
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

// expireQueued expires the settlement's actions still waiting for a signature.
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

func (u *UseCase) cancel(ctx context.Context, rules domain.Disclosed, tokens []domain.Disclosed, cid string) error {
	cmd, err := u.canton.CancelCommand(rules, tokens, cid, u.operator)
	if err != nil {
		return fmt.Errorf("canton.CancelCommand: %w", err)
	}

	_, err = u.canton.Submit(ctx, u.operator, "cancel:"+cid, cmd)
	if err != nil && !errors.Is(err, domain.ErrContractGone) && !errors.Is(err, domain.ErrDuplicateCommand) {
		return fmt.Errorf("canton.Submit cancel %s: %w", cid, err)
	}

	return nil
}
