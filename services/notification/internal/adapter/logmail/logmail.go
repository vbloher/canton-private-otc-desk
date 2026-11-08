package logmail

import (
	"context"
	"fmt"

	"github.com/rs/zerolog/log"

	"github.com/vbloher/canton-private-otc-desk/services/notification/internal/domain"
)

const providerID = "logged"

type Logmail struct{}

func New() *Logmail {
	return &Logmail{}
}

func (*Logmail) Send(ctx context.Context, e domain.Email) (string, error) {
	log.Info().Ctx(ctx).Str("to", e.Recipient).Str("subject", e.Subject).Str("text", e.Text).
		Msg("email: written to the log")

	return providerID, nil
}

func (*Logmail) Subscribe(ctx context.Context, email string) error {
	log.Info().Ctx(ctx).Str("email", email).Msg("email: contact subscribed in the log")

	return nil
}

func (*Logmail) Unsubscribe(ctx context.Context, email string) error {
	log.Info().Ctx(ctx).Str("email", email).Msg("email: contact unsubscribed in the log")

	return nil
}

func (*Logmail) ParseWebhook([]byte, string, string, string) (domain.ContactEvent, error) {
	return domain.ContactEvent{}, fmt.Errorf("%w: webhooks need Resend", domain.ErrUnauthorized)
}
