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

type CompleteRegistrationOutput struct {
	User  domain.User
	Token string
	TTL   time.Duration
}
