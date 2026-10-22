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

// LoginOutput carries the new session: Token goes into the cookie for TTL.
type LoginOutput struct {
	User  domain.User
	Token string
	TTL   time.Duration
}
