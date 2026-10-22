package usecase

import (
	"context"
	"errors"
	"fmt"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/domain"
)

// SeedHouseMakers registers the configured house makers with the party ids the ledger knows them by.
// It is idempotent and tries every maker; the error lists those that failed, so the caller can retry.
func (u *UseCase) SeedHouseMakers(ctx context.Context) error {
	ctx, span := tracer.Start(ctx, "usecase SeedHouseMakers")
	defer span.End()

	var errs []error

	for _, maker := range u.config.HouseMakers {
		err := u.seedHouseMaker(ctx, maker)
		if err != nil {
			errs = append(errs, fmt.Errorf("house maker %s: %w", maker.Hint, err))
		}
	}

	return errors.Join(errs...)
}

func (u *UseCase) seedHouseMaker(ctx context.Context, maker domain.HouseMaker) error {
	partyID, err := u.ledger.ResolveParty(ctx, maker.Hint)
	if err != nil {
		return fmt.Errorf("ledger.ResolveParty: %w", err)
	}

	user, err := domain.NewHouseUser(partyID, maker.Hint, maker.APIKey)
	if err != nil {
		return fmt.Errorf("domain.NewHouseUser: %w", err)
	}

	err = u.postgres.UpsertHouseUser(ctx, user)
	if err != nil {
		return fmt.Errorf("postgres.UpsertHouseUser: %w", err)
	}

	return nil
}
