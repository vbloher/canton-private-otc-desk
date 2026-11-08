package usecase

import (
	"context"
	"fmt"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/notification/internal/dto"
)

func (u *UseCase) HandleResendWebhook(ctx context.Context, input dto.ResendWebhookInput) error {
	ctx, span := tracer.Start(ctx, "usecase HandleResendWebhook")
	defer span.End()

	event, err := u.contacts.ParseWebhook(input.Payload, input.ID, input.Timestamp, input.Signature)
	if err != nil {
		return fmt.Errorf("contacts.ParseWebhook: %w", err)
	}

	if event.Email == "" || !event.Unsubscribed {
		return nil
	}

	parties, err := u.postgres.ClearNews(ctx, event.Email)
	if err != nil {
		return fmt.Errorf("postgres.ClearNews: %w", err)
	}

	if len(parties) > 0 {
		u.publishProfile(ctx, parties...)
	}

	return nil
}
