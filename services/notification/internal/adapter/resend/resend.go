package resend

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	resendsdk "github.com/resend/resend-go/v4"

	"github.com/vbloher/canton-private-otc-desk/pkg/metrics"
	"github.com/vbloher/canton-private-otc-desk/services/notification/internal/domain"
)

const (
	timeout        = 15 * time.Second
	contactUpdated = "contact.updated"
	contactsPath   = "contacts"
)

type Config struct {
	APIKey        string `env:"RESEND_API_KEY"`
	BaseURL       string `env:"RESEND_BASE_URL"`
	From          string `env:"EMAIL_FROM"`
	SegmentID     string `env:"RESEND_SEGMENT_ID"`
	WebhookSecret string `env:"RESEND_WEBHOOK_SECRET"`
}

type Resend struct {
	client *resendsdk.Client
	config Config
}

func New(c Config) (*Resend, error) {
	client := resendsdk.NewCustomClient(&http.Client{
		Timeout:   timeout,
		Transport: metrics.Transport(otelhttp.NewTransport(http.DefaultTransport, otelhttp.WithFilter(traced))),
	}, c.APIKey)

	if c.BaseURL != "" {
		base, err := url.Parse(c.BaseURL)
		if err != nil {
			return nil, fmt.Errorf("url.Parse: %w", err)
		}

		client.BaseURL = base
	}

	return &Resend{client: client, config: c}, nil
}

func traced(r *http.Request) bool {
	return !strings.HasPrefix(strings.TrimLeft(r.URL.Path, "/"), contactsPath)
}

func (r *Resend) ParseWebhook(payload []byte, id, timestamp, signature string) (domain.ContactEvent, error) {
	err := r.client.Webhooks.Verify(&resendsdk.VerifyWebhookOptions{
		Payload:       string(payload),
		Headers:       resendsdk.WebhookHeaders{Id: id, Timestamp: timestamp, Signature: signature},
		WebhookSecret: r.config.WebhookSecret,
	})
	if err != nil {
		return domain.ContactEvent{}, fmt.Errorf("%w: %w", domain.ErrUnauthorized, err)
	}

	var event struct {
		Type string `json:"type"`
		Data struct {
			Email        string `json:"email"`
			Unsubscribed bool   `json:"unsubscribed"`
		} `json:"data"`
	}

	err = json.Unmarshal(payload, &event)
	if err != nil {
		return domain.ContactEvent{}, fmt.Errorf("%w: unreadable webhook", domain.ErrValidation)
	}

	if event.Type != contactUpdated {
		return domain.ContactEvent{}, nil
	}

	return domain.ContactEvent{Email: strings.ToLower(event.Data.Email), Unsubscribed: event.Data.Unsubscribed}, nil
}

func sanitize(err error) error {
	if failed, ok := errors.AsType[*resendsdk.ResendError](err); ok {
		return fmt.Errorf("resend: status %d %s", failed.StatusCode, failed.Name)
	}

	if transport, ok := errors.AsType[*url.Error](err); ok {
		return fmt.Errorf("resend: %w", transport.Err)
	}

	return fmt.Errorf("resend: %w", err)
}

func statusIs(err error, statuses ...int) bool {
	failed, ok := errors.AsType[*resendsdk.ResendError](err)

	return ok && slices.Contains(statuses, failed.StatusCode)
}
