package resend

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	resendsdk "github.com/resend/resend-go/v4"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/notification/internal/domain"
)

func (r *Resend) Send(ctx context.Context, e domain.Email) (string, error) {
	ctx, span := tracer.Start(ctx, "adapter resend Send")
	defer span.End()

	sent, err := r.client.Emails.SendWithOptions(ctx, &resendsdk.SendEmailRequest{
		From:    r.config.From,
		To:      []string{e.Recipient},
		Subject: e.Subject,
		Html:    e.HTML,
		Text:    e.Text,
	}, &resendsdk.SendEmailOptions{IdempotencyKey: e.ID.String()})
	if err != nil {
		if undeliverable(err) {
			return "", fmt.Errorf("%w: %w", domain.ErrUndeliverable, sanitize(err))
		}

		return "", sanitize(err)
	}

	return sent.Id, nil
}

func undeliverable(err error) bool {
	failed, ok := errors.AsType[*resendsdk.ResendError](err)
	if !ok {
		return false
	}

	if statusIs(err, http.StatusUnauthorized, http.StatusForbidden, http.StatusRequestTimeout, http.StatusConflict) {
		return false
	}

	return failed.StatusCode >= http.StatusBadRequest && failed.StatusCode < http.StatusInternalServerError
}
