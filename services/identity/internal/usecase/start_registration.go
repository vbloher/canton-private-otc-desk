package usecase

import (
	"context"
	"fmt"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/dto"
)

func (u *UseCase) StartRegistration(
	ctx context.Context, input dto.StartRegistrationInput,
) (dto.StartRegistrationOutput, error) {
	ctx, span := tracer.Start(ctx, "usecase StartRegistration")
	defer span.End()

	var output dto.StartRegistrationOutput

	reg, err := domain.NewRegistration(input.DisplayName, input.PublicKey)
	if err != nil {
		return output, fmt.Errorf("domain.NewRegistration: %w", err)
	}

	topology, err := u.ledger.GenerateExternalParty(ctx, reg.Hint, reg.PublicKey)
	if err != nil {
		return output, fmt.Errorf("ledger.GenerateExternalParty: %w", err)
	}

	reg.Topology = topology

	err = u.redis.SaveRegistration(ctx, reg, registrationTTL)
	if err != nil {
		return output, fmt.Errorf("redis.SaveRegistration: %w", err)
	}

	return dto.StartRegistrationOutput{
		RegistrationID: reg.ID,
		PartyID:        topology.PartyID,
		MultiHash:      topology.MultiHash,
	}, nil
}
