package dto

import "github.com/google/uuid"

type StartRegistrationInput struct {
	DisplayName string
	PublicKey   []byte
}

type StartRegistrationOutput struct {
	RegistrationID uuid.UUID
	PartyID        string
	MultiHash      []byte
}
