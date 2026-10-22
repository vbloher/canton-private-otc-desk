package usecase

import (
	"context"
	"crypto/ed25519"
	"fmt"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/dto"
)

// GenerateExternalParty is step 1 of onboarding a browser-held key: the topology it signs.
func (u *UseCase) GenerateExternalParty(
	ctx context.Context, input dto.GenerateExternalPartyInput,
) (dto.GenerateExternalPartyOutput, error) {
	ctx, span := tracer.Start(ctx, "usecase GenerateExternalParty")
	defer span.End()

	var output dto.GenerateExternalPartyOutput

	if input.Hint == "" || len(input.PublicKey) != ed25519.PublicKeySize {
		return output, domain.Errorf(domain.ErrValidation, "a hint and a %d-byte Ed25519 public key are required",
			ed25519.PublicKeySize)
	}

	synchronizerID, err := u.synchronizerID(ctx)
	if err != nil {
		return output, fmt.Errorf("u.synchronizerID: %w", err)
	}

	topology, err := u.canton.GenerateExternalParty(ctx, synchronizerID, input.Hint, input.PublicKey)
	if err != nil {
		return output, fmt.Errorf("canton.GenerateExternalParty: %w", err)
	}

	return dto.GenerateExternalPartyOutput{Topology: topology}, nil
}
