package usecase

import (
	"context"
	"crypto/ed25519"
	"fmt"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/dto"
)

func (u *UseCase) AllocateExternalParty(
	ctx context.Context, input dto.AllocateExternalPartyInput,
) (dto.AllocateExternalPartyOutput, error) {
	ctx, span := tracer.Start(ctx, "usecase AllocateExternalParty")
	defer span.End()

	var output dto.AllocateExternalPartyOutput

	if len(input.Transactions) == 0 || len(input.Signature) != ed25519.SignatureSize || input.Fingerprint == "" {
		return output, domain.Errorf(domain.ErrValidation,
			"topology transactions, a %d-byte signature and the fingerprint are required", ed25519.SignatureSize)
	}

	synchronizerID, err := u.synchronizerID(ctx)
	if err != nil {
		return output, fmt.Errorf("u.synchronizerID: %w", err)
	}

	partyID, err := u.canton.AllocateExternalParty(ctx, synchronizerID, input.Transactions, input.Signature,
		input.Fingerprint)
	if err != nil {
		return output, fmt.Errorf("canton.AllocateExternalParty: %w", err)
	}

	return dto.AllocateExternalPartyOutput{PartyID: partyID}, nil
}
