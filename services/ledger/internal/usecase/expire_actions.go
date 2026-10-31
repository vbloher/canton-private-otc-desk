package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/dto"
)

func (u *UseCase) ExpireActions(ctx context.Context) (dto.ExpireActionsOutput, error) {
	expired, err := u.postgres.ExpireActions(ctx, time.Now())
	if err != nil {
		return dto.ExpireActionsOutput{}, fmt.Errorf("postgres.ExpireActions: %w", err)
	}

	if len(expired) == 0 {
		return dto.ExpireActionsOutput{}, nil
	}

	ctx, span := tracer.StartRoot(ctx, "usecase ExpireActions")
	defer span.End()

	for _, action := range expired {
		u.publish(ctx, signCompleted(action.ID), action.PartyID)
	}

	return dto.ExpireActionsOutput{Expired: len(expired)}, nil
}
