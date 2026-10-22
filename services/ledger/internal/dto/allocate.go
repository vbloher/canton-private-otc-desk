package dto

import (
	"time"

	"github.com/google/uuid"
)

type AllocateInput struct {
	SettlementID string
	Deadline     time.Time
	Legs         []TransferLeg
	PartyID      string
	Description  string
}

type AllocateOutput struct {
	Executed  bool // the desk submitted the allocations itself (hosted party)
	ActionIDs []uuid.UUID
}
