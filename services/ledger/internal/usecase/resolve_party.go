package usecase

import (
	"context"
	"fmt"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/dto"
)

func (u *UseCase) ResolveParty(ctx context.Context, input dto.ResolvePartyInput) (dto.ResolvePartyOutput, error) {
	ctx, span := tracer.Start(ctx, "usecase ResolveParty")
	defer span.End()

	var output dto.ResolvePartyOutput

	if input.Hint == "" {
		return output, domain.Errorf(domain.ErrValidation, "hint is required")
	}

	partyID, err := u.canton.ResolveParty(ctx, input.Hint)
	if err != nil {
		return output, fmt.Errorf("canton.ResolveParty: %w", err)
	}

	return dto.ResolvePartyOutput{PartyID: partyID}, nil
}
