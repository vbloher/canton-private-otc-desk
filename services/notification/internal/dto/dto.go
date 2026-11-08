package dto

import "github.com/vbloher/canton-private-otc-desk/services/notification/internal/domain"

type GetProfileInput struct {
	PartyID string
}

type ProfileOutput struct {
	Profile      domain.Profile
	EmailEnabled bool
}

type SetEmailInput struct {
	PartyID string
	Email   string
}

type ConfirmEmailInput struct {
	Token string
}

type SetPreferencesInput struct {
	PartyID string
	News    bool
	Trades  bool
}

type ResendWebhookInput struct {
	Payload   []byte
	ID        string
	Timestamp string
	Signature string
}
