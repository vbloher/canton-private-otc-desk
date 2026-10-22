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

// Purposes of a sign action, as the wallet API shows them.
const (
	PurposeAllocate       = "allocate"
	PurposeTransferAccept = "transfer-accept"
	PurposeTransferOut    = "transfer-out"
)

// Intent is what a party's command is for: Ref names what it acts on (a settlement id, a
// transfer contract id, or a withdrawal's receiver) and Counterparty is the other party
// whose wallet its outcome changes (empty when there is none).
type Intent struct {
	PartyID      string
	Purpose      string
	Description  string
	Ref          string
	Counterparty string
}

// Action is a prepared transaction of an external party waiting for its signature.
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

// Duplicates reports whether the action already stands for the intent, so it is handed back
// instead of queuing another one. A withdrawal's ref is only its receiver: it duplicates only
// a withdrawal of the same amount of the same instrument, which its description names.
func (a Action) Duplicates(in Intent) bool {
	if a.PartyID != in.PartyID || a.Purpose != in.Purpose || a.Ref != in.Ref {
		return false
	}

	return a.Purpose != PurposeTransferOut || a.Description == in.Description
}

// Executable checks that the party may sign the action now. Another party's action does
// not exist for it.
func (a Action) Executable(partyID string, now time.Time) error {
	if a.PartyID != partyID {
		return ErrNotFound
	}

	if a.Status != ActionPending || !now.Before(a.ExpiresAt) {
		return ErrActionNotPending
	}

	return nil
}

// Outcome is the action after the ledger answered its execution, and whether to record it.
// Committed, or a duplicate of a command that has already landed, is executed. Nothing was
// decided when the ledger was unavailable, or when the signature was made with another key
// (another user's tab): the action stays pending for its party to sign. Any other rejection
// failed it.
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

// Finished records the outcome of executing the action.
func (a Action) Finished(err error) Action {
	if err != nil {
		a.Status = ActionFailed
		a.Error = err.Error()

		return a
	}

	a.Status = ActionExecuted

	return a
}
