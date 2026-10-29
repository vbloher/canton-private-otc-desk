package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/dto"
)

func (u *UseCase) ExpireActions(ctx context.Context) (dto.ExpireActionsOutput, error) {
	ctx, span := tracer.Start(ctx, "usecase ExpireActions")
	defer span.End()

	var output dto.ExpireActionsOutput

	expired, err := u.postgres.ExpireActions(ctx, time.Now())
	if err != nil {
		return output, fmt.Errorf("postgres.ExpireActions: %w", err)
	}

	for _, action := range expired {
		u.publish(ctx, signCompleted(action.ID), action.PartyID)
	}

	return dto.ExpireActionsOutput{Expired: len(expired)}, nil
}
