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

type LoginOutput struct {
	User  domain.User
	Token string
	TTL   time.Duration
}
