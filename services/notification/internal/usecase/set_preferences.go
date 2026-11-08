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

func (u *UseCase) SetPreferences(ctx context.Context, input dto.SetPreferencesInput) (dto.ProfileOutput, error) {
	ctx, span := tracer.Start(ctx, "usecase SetPreferences")
	defer span.End()

	if !u.config.Enabled {
		return dto.ProfileOutput{}, domain.ErrEmailDisabled
	}

	var profile domain.Profile

	err := u.tx.Wrap(ctx, func(ctx context.Context) error {
		current, err := u.postgres.GetProfileForUpdate(ctx, input.PartyID)
		if errors.Is(err, domain.ErrNotFound) {
			return domain.ErrEmailNotConfirmed
		}

		if err != nil {
			return fmt.Errorf("postgres.GetProfileForUpdate: %w", err)
		}

		if !current.Confirmed() {
			return domain.ErrEmailNotConfirmed
		}

		newsChanged := current.News != input.News

		current.News = input.News
		current.Trades = input.Trades
		profile = current

		err = u.postgres.SaveProfile(ctx, current)
		if err != nil {
			return fmt.Errorf("postgres.SaveProfile: %w", err)
		}

		if newsChanged {
			err = u.postgres.QueueContactSync(ctx, current.Email, time.Now())
			if err != nil {
				return fmt.Errorf("postgres.QueueContactSync: %w", err)
			}
		}

		return nil
	})
	if err != nil {
		return dto.ProfileOutput{}, fmt.Errorf("transaction.Wrap: %w", err)
	}

	u.publishProfile(ctx, input.PartyID)

	return dto.ProfileOutput{Profile: profile, EmailEnabled: true}, nil
}
