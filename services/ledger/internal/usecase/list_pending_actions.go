package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/dto"
)

func (u *UseCase) ListPendingActions(
	ctx context.Context, input dto.ListPendingActionsInput,
) (dto.ListPendingActionsOutput, error) {
	ctx, span := tracer.Start(ctx, "usecase ListPendingActions")
	defer span.End()

	var output dto.ListPendingActionsOutput

	actions, err := u.postgres.ListPendingActions(ctx, input.PartyID, time.Now())
	if err != nil {
		return output, fmt.Errorf("postgres.ListPendingActions: %w", err)
	}

	return dto.ListPendingActionsOutput{Actions: actions}, nil
}
