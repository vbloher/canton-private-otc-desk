package usecase

import (
	"context"
	"fmt"
	"strings"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/domain"
)

func (u *UseCase) SeedHouseMaker(ctx context.Context) error {
	ctx, span := tracer.Start(ctx, "usecase SeedHouseMaker")
	defer span.End()

	if u.config.MakerParty == "" {
		return nil
	}

	partyID, err := u.ledger.ResolveParty(ctx, u.config.MakerParty)
	if err != nil {
		return fmt.Errorf("ledger.ResolveParty: %w", err)
	}

	hint, _, _ := strings.Cut(partyID, "::")

	user, err := domain.NewHouseUser(partyID, hint, u.config.MakerAPIKey)
	if err != nil {
		return fmt.Errorf("domain.NewHouseUser: %w", err)
	}

	err = u.postgres.UpsertHouseUser(ctx, user)
	if err != nil {
		return fmt.Errorf("postgres.UpsertHouseUser: %w", err)
	}

	return nil
}
