package dto

import (
	"time"

	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/domain"
)

type LoginInput struct {
	Challenge    string
	Signature    []byte
	CredentialID string
}

// LoginOutput carries the new session. Token is the secret: it goes into the cookie for TTL, and is
// also handed to clients that send it as a bearer token. ExpiresAt is the expiry that is stored.
type LoginOutput struct {
	User      domain.User
	Token     string
	TTL       time.Duration
	ExpiresAt time.Time
}
