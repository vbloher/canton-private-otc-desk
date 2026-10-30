package dto

import (
	"fmt"

	"github.com/google/uuid"

	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/domain"
)

type RegisterPartyInput struct {
	RegistrationID uuid.UUID
	Signature      []byte
}

func (i RegisterPartyInput) Validate() error {
	err := domain.ValidateSignature(i.Signature)
	if err != nil {
		return fmt.Errorf("domain.ValidateSignature: %w", err)
	}

	return nil
}

type RegisterPartyOutput struct {
	PartyID string
}
