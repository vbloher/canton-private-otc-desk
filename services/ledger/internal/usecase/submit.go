package usecase

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/domain"
)

// submit is the one place deciding how a party's command reaches the ledger. The desk
// submits it itself for a party it hosts (nil action). An external party is hosted with
// confirmation rights only, so its command is prepared and comes back as a pending action
// for its key to sign; done saves and announces it. Either way the command id is the
// ledger's change id: under a deterministic id a command lands once, and a re-submission
// answered with DUPLICATE_COMMAND has already landed.
func (u *UseCase) submit(ctx context.Context, in domain.Intent, cmd domain.Command, commandID string,
) (*domain.Action, error) {
	if u.hosted[in.PartyID] {
		_, err := u.canton.Submit(ctx, in.PartyID, commandID, cmd)
		if err != nil && !errors.Is(err, domain.ErrDuplicateCommand) {
			return nil, fmt.Errorf("canton.Submit: %w", err)
		}

		return nil, nil //nolint:nilnil // a hosted party's command needs no signature
	}

	prepared, err := u.canton.Prepare(ctx, in.PartyID, commandID, cmd)
	if err != nil {
		return nil, fmt.Errorf("canton.Prepare: %w", err)
	}

	action := domain.NewAction(in, prepared, time.Now(), u.config.ActionTTL)

	return &action, nil
}

// done finishes what submit started for a party: an external party's actions are saved in
// one batch and announced; for a hosted party the ledger has already changed, for it and for
// the counterparties.
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

// queued are an external party's pending actions that already stand for the intent: a
// retried request gets them back instead of queuing duplicates. A hosted party has none;
// its retries are deduplicated by the ledger.
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

// collect appends a submitted command's action, if it needs a signature.
func collect(actions []domain.Action, action *domain.Action) []domain.Action {
	if action == nil {
		return actions
	}

	return append(actions, *action)
}

func nonEmpty(ids ...string) []string {
	return slices.DeleteFunc(ids, func(id string) bool { return id == "" })
}
