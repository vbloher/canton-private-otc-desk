package dto

import (
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/domain"
)

type CompleteRegistrationInput struct {
	RegistrationID uuid.UUID
	Signature      []byte
	CredentialID   string
	EncryptedBlob  string
}

// Validate refuses a request that cannot become a wallet, without touching anything: the checks
// that need the parked registration or the ledger come later.
func (i CompleteRegistrationInput) Validate() error {
	err := domain.ValidateSignature(i.Signature)
	if err != nil {
		return fmt.Errorf("domain.ValidateSignature: %w", err)
	}

	err = domain.ValidateCredential(i.CredentialID, i.EncryptedBlob)
	if err != nil {
		return fmt.Errorf("domain.ValidateCredential: %w", err)
	}

	return nil
}

// CompleteRegistrationOutput signs the new wallet in. Token is the secret: it goes into the cookie
// for TTL, and is also handed to clients that send it as a bearer token. ExpiresAt is the expiry
// that is stored.
type CompleteRegistrationOutput struct {
	User      domain.User
	Token     string
	TTL       time.Duration
	ExpiresAt time.Time
}
