package domain

import (
	"errors"
	"fmt"

	"github.com/vbloher/canton-private-otc-desk/pkg/decimal"
)

var (
	ErrNotFound          = errors.New("not found")
	ErrValidation        = errors.New("validation failed")
	ErrInsufficientFunds = errors.New("insufficient funds")
	ErrActionNotPending  = errors.New("action is not pending")
	ErrUnknownInstrument = errors.New("unknown instrument")

	// ErrLedger wraps every Canton failure. The kinds below come wrapped in it too: a
	// duplicate command has already landed, a gone contract was consumed by another
	// transaction (a cancel counts it as done, anything else may retry with fresh state),
	// and an unavailable ledger may answer a retry of the same request.
	ErrLedger            = errors.New("ledger failed")
	ErrDuplicateCommand  = errors.New("command already submitted")
	ErrContractGone      = errors.New("contract is no longer active")
	ErrLedgerUnavailable = errors.New("ledger unavailable")

	// ErrAmbiguousParty: one hint names several parties (a ledger seeded twice).
	ErrAmbiguousParty = errors.New("party hint is ambiguous")

	// ErrSignatureRejected: the ledger found no valid signature of the action's party over
	// its hash (signed with another key). The prepared transaction itself is still good.
	ErrSignatureRejected = errors.New("signature does not match this action's party")
)

// Errorf wraps kind with the details as a second wrapped error: errors.Unwrap stops at the
// wrapper, so pkg/render shows the client the whole message, and errors.Is still finds kind.
func Errorf(kind error, format string, args ...any) error {
	return fmt.Errorf("%w: %w", kind, fmt.Errorf(format, args...))
}

// Invalidf reports what is wrong with the input.
func Invalidf(format string, args ...any) error {
	return Errorf(ErrValidation, format, args...)
}

// InsufficientFundsError says which instrument fell short; it matches ErrInsufficientFunds.
type InsufficientFundsError struct {
	Instrument string
	Need       decimal.Decimal
	Held       decimal.Decimal
}

func (e InsufficientFundsError) Error() string {
	return fmt.Sprintf("%s: %s: need %s, held %s", ErrInsufficientFunds, e.Instrument, e.Need, e.Held)
}

func (e InsufficientFundsError) Is(target error) bool {
	return target == ErrInsufficientFunds
}

func IsInvalidInput(err error) bool {
	return errors.Is(err, ErrValidation) ||
		errors.Is(err, ErrUnknownInstrument) ||
		errors.Is(err, ErrSignatureRejected)
}
