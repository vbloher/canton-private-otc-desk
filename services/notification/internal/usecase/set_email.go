package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/notification/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/notification/internal/dto"
)

func (u *UseCase) SetEmail(ctx context.Context, input dto.SetEmailInput) (dto.ProfileOutput, error) {
	ctx, span := tracer.Start(ctx, "usecase SetEmail")
	defer span.End()

	if !u.config.Enabled {
		return dto.ProfileOutput{}, domain.ErrEmailDisabled
	}

	email, err := domain.NormalizeEmail(input.Email)
	if err != nil {
		return dto.ProfileOutput{}, fmt.Errorf("domain.NormalizeEmail: %w", err)
	}

	found, err := u.postgres.GetProfile(ctx, input.PartyID)

	current, err := orEmpty(input.PartyID, found, err)
	if err != nil {
		return dto.ProfileOutput{}, fmt.Errorf("postgres.GetProfile: %w", err)
	}

	if current.Email == email && current.Confirmed() {
		return dto.ProfileOutput{Profile: current, EmailEnabled: true}, nil
	}

	allowed, err := u.redis.AllowConfirmation(ctx, input.PartyID, email)
	if err != nil {
		return dto.ProfileOutput{}, fmt.Errorf("redis.AllowConfirmation: %w", err)
	}

	if !allowed {
		return dto.ProfileOutput{}, domain.ErrTooManyRequests
	}

	profile, err := u.storeEmail(ctx, input.PartyID, email, time.Now())
	if err != nil {
		return dto.ProfileOutput{}, err
	}

	return dto.ProfileOutput{Profile: profile, EmailEnabled: true}, nil
}

func (u *UseCase) storeEmail(ctx context.Context, partyID, email string, now time.Time) (domain.Profile, error) {
	var profile domain.Profile

	err := u.tx.Wrap(ctx, func(ctx context.Context) error {
		found, err := u.postgres.GetProfileForUpdate(ctx, partyID)

		previous, err := orEmpty(partyID, found, err)
		if err != nil {
			return fmt.Errorf("postgres.GetProfileForUpdate: %w", err)
		}

		profile = previous.WithEmail(email)

		err = u.postgres.SaveProfile(ctx, profile)
		if err != nil {
			return fmt.Errorf("postgres.SaveProfile: %w", err)
		}

		err = u.queueConfirmation(ctx, partyID, email, now)
		if err != nil {
			return err
		}

		if previous.News && previous.Confirmed() && previous.Email != email {
			err = u.postgres.QueueContactSync(ctx, previous.Email, now)
			if err != nil {
				return fmt.Errorf("postgres.QueueContactSync: %w", err)
			}
		}

		return nil
	})
	if err != nil {
		return domain.Profile{}, fmt.Errorf("transaction.Wrap: %w", err)
	}

	return profile, nil
}

func (u *UseCase) queueConfirmation(ctx context.Context, partyID, email string, now time.Time) error {
	confirmation, token := domain.NewConfirmation(partyID, email, now, u.config.ConfirmationTTL)

	err := u.postgres.ReplaceConfirmation(ctx, confirmation)
	if err != nil {
		return fmt.Errorf("postgres.ReplaceConfirmation: %w", err)
	}

	letter, err := domain.ConfirmationLetter(u.config.baseURL()+"/confirm-email#token="+token, u.config.ConfirmationTTL)
	if err != nil {
		return fmt.Errorf("domain.ConfirmationLetter: %w", err)
	}

	err = u.postgres.CreateEmail(ctx,
		domain.NewEmail("confirmation:"+confirmation.TokenHash, email, domain.KindConfirmation, letter, now))
	if err != nil {
		return fmt.Errorf("postgres.CreateEmail: %w", err)
	}

	return nil
}
