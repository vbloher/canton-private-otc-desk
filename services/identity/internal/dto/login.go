package dto

import (
	"fmt"
	"time"

	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/domain"
)

type LoginInput struct {
	Challenge    string
	Signature    []byte
	CredentialID string
	PartyID      string
}

func (i LoginInput) Validate() error {
	err := domain.ValidateIdentifier(i.CredentialID, i.PartyID)
	if err != nil {
		return fmt.Errorf("domain.ValidateIdentifier: %w", err)
	}

	return nil
}

type LoginOutput struct {
	User  domain.User
	Token string
	TTL   time.Duration
}
