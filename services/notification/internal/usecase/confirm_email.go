package usecase

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/notification/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/notification/internal/dto"
)

func (u *UseCase) ConfirmEmail(ctx context.Context, input dto.ConfirmEmailInput) error {
	ctx, span := tracer.Start(ctx, "usecase ConfirmEmail")
	defer span.End()

	if input.Token == "" {
		return domain.ErrConfirmationInvalid
	}

	hash := domain.HashToken(input.Token)

	first, err := u.lookup(ctx, hash)
	if err != nil {
		return err
	}

	var changed bool

	err = u.tx.Wrap(ctx, func(ctx context.Context) error {
		profile, err := u.postgres.GetProfileForUpdate(ctx, first.PartyID)
		if err != nil {
			return fmt.Errorf("postgres.GetProfileForUpdate: %w", err)
		}

		confirmation, err := u.lookup(ctx, hash)
		if err != nil {
			return err
		}

		changed, err = u.confirm(ctx, confirmation, profile, time.Now())

		return err
	})
	if err != nil {
		return fmt.Errorf("transaction.Wrap: %w", err)
	}

	if changed {
		u.publishProfile(ctx, first.PartyID)
	}

	return nil
}

func (u *UseCase) lookup(ctx context.Context, hash string) (domain.Confirmation, error) {
	confirmation, err := u.postgres.GetConfirmation(ctx, hash)
	if errors.Is(err, domain.ErrNotFound) {
		return domain.Confirmation{}, domain.ErrConfirmationInvalid
	}

	if err != nil {
		return domain.Confirmation{}, fmt.Errorf("postgres.GetConfirmation: %w", err)
	}

	return confirmation, nil
}

func (u *UseCase) confirm(
	ctx context.Context, c domain.Confirmation, profile domain.Profile, now time.Time,
) (bool, error) {
	if profile.Email != c.Email {
		return false, domain.ErrConfirmationInvalid
	}

	if c.ConfirmedAt != nil {
		return false, nil
	}

	if c.Expired(now) {
		return false, domain.ErrConfirmationInvalid
	}

	err := u.postgres.MarkConfirmed(ctx, c.TokenHash, now)
	if err != nil {
		return false, fmt.Errorf("postgres.MarkConfirmed: %w", err)
	}

	profile.ConfirmedAt = &now

	err = u.postgres.SaveProfile(ctx, profile)
	if err != nil {
		return false, fmt.Errorf("postgres.SaveProfile: %w", err)
	}

	if profile.News {
		err = u.postgres.QueueContactSync(ctx, profile.Email, now)
		if err != nil {
			return false, fmt.Errorf("postgres.QueueContactSync: %w", err)
		}
	}

	return true, nil
}
