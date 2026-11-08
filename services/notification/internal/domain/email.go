package domain

import (
	"time"

	"github.com/google/uuid"
)

const (
	MaxAttempts = 10

	firstRetry    = 30 * time.Second
	maxRetry      = time.Hour
	maxErrorChars = 500
)

type EmailKind string

const (
	KindConfirmation EmailKind = "confirmation"
	KindTradeSettled EmailKind = "trade_settled"
)

type DeliveryStatus string

const (
	DeliveryPending DeliveryStatus = "pending"
	DeliverySent    DeliveryStatus = "sent"
	DeliveryFailed  DeliveryStatus = "failed"
)

type Email struct {
	ID            uuid.UUID
	DedupeKey     string
	Recipient     string
	Kind          EmailKind
	Subject       string
	HTML          string
	Text          string
	Status        DeliveryStatus
	Attempts      int
	NextAttemptAt time.Time
	ProviderID    string
	LastError     string
	CreatedAt     time.Time
	FinishedAt    *time.Time
}

func NewEmail(dedupeKey, recipient string, kind EmailKind, letter Letter, now time.Time) Email {
	return Email{
		ID:            uuid.New(),
		DedupeKey:     dedupeKey,
		Recipient:     recipient,
		Kind:          kind,
		Subject:       letter.Subject,
		HTML:          letter.HTML,
		Text:          letter.Text,
		Status:        DeliveryPending,
		NextAttemptAt: now,
		CreatedAt:     now,
	}
}

func RetryDelay(attempts int) time.Duration {
	delay := firstRetry

	for range attempts - 1 {
		delay *= 2

		if delay >= maxRetry {
			return maxRetry
		}
	}

	return delay
}

func (e Email) Delivered(providerID string, now time.Time) Email {
	e.Status = DeliverySent
	e.ProviderID = providerID

	return e.finished(now)
}

func (e Email) Undeliverable(reason string, now time.Time) Email {
	e.Status = DeliveryFailed
	e.LastError = shorten(reason)

	return e.finished(now)
}

func (e Email) Retry(reason string, now time.Time) Email {
	e.Attempts++
	e.LastError = shorten(reason)

	if e.Attempts >= MaxAttempts {
		e.Status = DeliveryFailed

		return e.finished(now)
	}

	e.NextAttemptAt = now.Add(RetryDelay(e.Attempts))

	return e
}

func (e Email) finished(now time.Time) Email {
	e.Recipient = ""
	e.Subject = ""
	e.HTML = ""
	e.Text = ""
	e.FinishedAt = &now

	return e
}

func shorten(reason string) string {
	if len(reason) > maxErrorChars {
		return reason[:maxErrorChars]
	}

	return reason
}
