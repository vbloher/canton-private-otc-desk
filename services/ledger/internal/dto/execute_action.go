package dto

import "github.com/google/uuid"

type ExecuteActionInput struct {
	ID        uuid.UUID
	PartyID   string
	Signature []byte // raw Ed25519 over the action's hash
}
