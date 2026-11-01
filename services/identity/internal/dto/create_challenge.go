package dto

import (
	"fmt"

	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/domain"
)

type CreateChallengeInput struct {
	CredentialID string
	PartyID      string
}

func (i CreateChallengeInput) Validate() error {
	err := domain.ValidateIdentifier(i.CredentialID, i.PartyID)
	if err != nil {
		return fmt.Errorf("domain.ValidateIdentifier: %w", err)
	}

	return nil
}

type CreateChallengeOutput struct {
	Challenge     string
	EncryptedBlob string
	PublicKey     []byte
}
