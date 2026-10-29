package domain

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

type ActionStatus string

const (
	ActionPending  ActionStatus = "pending"
	ActionExecuted ActionStatus = "executed"
	ActionFailed   ActionStatus = "failed"
	ActionExpired  ActionStatus = "expired"
)

const (
	PurposeAllocate       = "allocate"
	PurposeTransferAccept = "transfer-accept"
	PurposeTransferOut    = "transfer-out"
)

type Intent struct {
	PartyID      string
	Purpose      string
	Description  string
	Ref          string
	Counterparty string
}

type Action struct {
	ID           uuid.UUID
	PartyID      string
	Purpose      string
	Description  string
	Ref          string
	Counterparty string
	Hash         []byte
	Prepared     Prepared
	Status       ActionStatus
	Error        string
	CreatedAt    time.Time
	ExpiresAt    time.Time
}

func NewAction(in Intent, prepared Prepared, now time.Time, ttl time.Duration) Action {
	return Action{
		ID:           uuid.New(),
		PartyID:      in.PartyID,
		Purpose:      in.Purpose,
		Description:  in.Description,
		Ref:          in.Ref,
		Counterparty: in.Counterparty,
		Hash:         prepared.Hash,
		Prepared:     prepared,
		Status:       ActionPending,
		CreatedAt:    now,
		ExpiresAt:    now.Add(ttl),
	}
}

func (a Action) Duplicates(in Intent) bool {
	if a.PartyID != in.PartyID || a.Purpose != in.Purpose || a.Ref != in.Ref {
		return false
	}

	return a.Purpose != PurposeTransferOut || a.Description == in.Description
}

func (a Action) Executable(partyID string, now time.Time) error {
	if a.PartyID != partyID {
		return ErrNotFound
	}

	if a.Status != ActionPending || !now.Before(a.ExpiresAt) {
		return ErrActionNotPending
	}

	return nil
}

func (a Action) Outcome(err error) (Action, bool) {
	switch {
	case err == nil || errors.Is(err, ErrDuplicateCommand):
		return a.Finished(nil), true
	case errors.Is(err, ErrLedgerUnavailable), errors.Is(err, ErrSignatureRejected):
		return a, false
	default:
		return a.Finished(err), true
	}
}

func (a Action) Finished(err error) Action {
	if err != nil {
		a.Status = ActionFailed
		a.Error = err.Error()

		return a
	}

	a.Status = ActionExecuted

	return a
}
