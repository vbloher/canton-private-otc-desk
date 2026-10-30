package usecase

import (
	"context"
	"fmt"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/dto"
)

func (u *UseCase) RegisterParty(ctx context.Context, input dto.RegisterPartyInput) (dto.RegisterPartyOutput, error) {
	ctx, span := tracer.Start(ctx, "usecase RegisterParty")
	defer span.End()

	var output dto.RegisterPartyOutput

	err := input.Validate()
	if err != nil {
		return output, fmt.Errorf("input.Validate: %w", err)
	}

	reg, err := u.redis.GetRegistration(ctx, input.RegistrationID)
	if err != nil {
		return output, fmt.Errorf("redis.GetRegistration: %w", err)
	}

	err = reg.VerifySignature(input.Signature)
	if err != nil {
		return output, fmt.Errorf("reg.VerifySignature: %w", err)
	}

	if reg.PartyID != "" {
		return dto.RegisterPartyOutput{PartyID: reg.PartyID}, nil
	}

	reg.PartyID, err = u.partyOf(ctx, reg, input.Signature)
	if err != nil {
		return output, err
	}

	err = u.redis.SaveRegistration(ctx, reg, registrationTTL)
	if err != nil {
		return output, fmt.Errorf("redis.SaveRegistration: %w", err)
	}

	return dto.RegisterPartyOutput{PartyID: reg.PartyID}, nil
}

func (u *UseCase) partyOf(ctx context.Context, reg domain.Registration, signature []byte) (string, error) {
	if reg.PartyID != "" {
		return reg.PartyID, nil
	}

	partyID, err := u.ledger.AllocateExternalParty(ctx, reg.Topology, signature)
	if err != nil {
		return "", fmt.Errorf("ledger.AllocateExternalParty: %w", err)
	}

	return partyID, nil
}
