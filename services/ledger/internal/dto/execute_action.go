package dto

import "github.com/google/uuid"

type ExecuteActionInput struct {
	ID        uuid.UUID
	PartyID   string
	Signature []byte
}
