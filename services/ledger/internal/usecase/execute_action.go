package usecase

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"time"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/dto"
)

func (u *UseCase) ExecuteAction(ctx context.Context, input dto.ExecuteActionInput) error {
	ctx, span := tracer.Start(ctx, "usecase ExecuteAction")
	defer span.End()

	if len(input.Signature) != ed25519.SignatureSize {
		return domain.Errorf(domain.ErrValidation, "the signature must be %d bytes, got %d",
			ed25519.SignatureSize, len(input.Signature))
	}

	ctx = context.WithoutCancel(ctx)

	var (
		action     domain.Action
		executeErr error
	)

	err := u.tx.Wrap(ctx, func(ctx context.Context) error {
		var err error

		action, err = u.postgres.GetActionForUpdate(ctx, input.ID)
		if err != nil {
			return fmt.Errorf("postgres.GetActionForUpdate: %w", err)
		}

		err = action.Executable(input.PartyID, time.Now())
		if err != nil {
			return fmt.Errorf("action.Executable: %w", err)
		}

		executeErr = u.canton.Execute(ctx, action.Prepared, action.PartyID, input.Signature)

		var record bool

		action, record = action.Outcome(executeErr)
		if !record {
			return nil
		}

		err = u.postgres.UpdateAction(ctx, action)
		if err != nil {
			return fmt.Errorf("postgres.UpdateAction: %w", err)
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("transaction.Wrap: %w", err)
	}

	u.announce(ctx, action)

	if action.Status != domain.ActionExecuted {
		return fmt.Errorf("canton.Execute: %w", executeErr)
	}

	return nil
}

func (u *UseCase) announce(ctx context.Context, action domain.Action) {
	if action.Status == domain.ActionPending {
		return
	}

	u.publish(ctx, signCompleted(action.ID), action.PartyID)

	if action.Status == domain.ActionExecuted {
		u.publish(ctx, walletUpdated(), nonEmpty(action.PartyID, action.Counterparty)...)
	}
}
