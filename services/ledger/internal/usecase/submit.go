package usecase

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/domain"
)

func (u *UseCase) submit(ctx context.Context, in domain.Intent, cmd domain.Command, commandID string,
) (*domain.Action, error) {
	if u.hosted[in.PartyID] {
		_, err := u.canton.Submit(ctx, in.PartyID, commandID, cmd)
		if err != nil && !errors.Is(err, domain.ErrDuplicateCommand) {
			return nil, fmt.Errorf("canton.Submit: %w", err)
		}

		return nil, nil //nolint:nilnil
	}

	prepared, err := u.canton.Prepare(ctx, in.PartyID, commandID, cmd)
	if err != nil {
		return nil, fmt.Errorf("canton.Prepare: %w", err)
	}

	action := domain.NewAction(in, prepared, time.Now(), u.config.ActionTTL)

	return &action, nil
}

func (u *UseCase) done(ctx context.Context, party string, actions []domain.Action, counterparties ...string) error {
	if len(actions) == 0 {
		u.publish(ctx, walletUpdated(), nonEmpty(append([]string{party}, counterparties...)...)...)

		return nil
	}

	err := u.postgres.SaveActions(ctx, actions...)
	if err != nil {
		return fmt.Errorf("postgres.SaveActions: %w", err)
	}

	for _, action := range actions {
		u.publish(ctx, signRequested(action.ID), action.PartyID)
	}

	return nil
}

func (u *UseCase) queued(ctx context.Context, in domain.Intent) ([]domain.Action, error) {
	if u.hosted[in.PartyID] {
		return nil, nil
	}

	pending, err := u.postgres.ListPendingActions(ctx, in.PartyID, time.Now())
	if err != nil {
		return nil, fmt.Errorf("postgres.ListPendingActions: %w", err)
	}

	var same []domain.Action

	for _, action := range pending {
		if action.Duplicates(in) {
			same = append(same, action)
		}
	}

	return same, nil
}

func collect(actions []domain.Action, action *domain.Action) []domain.Action {
	if action == nil {
		return actions
	}

	return append(actions, *action)
}

func nonEmpty(ids ...string) []string {
	return slices.DeleteFunc(ids, func(id string) bool { return id == "" })
}
