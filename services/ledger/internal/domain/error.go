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

	ErrLedger            = errors.New("ledger failed")
	ErrDuplicateCommand  = errors.New("command already submitted")
	ErrContractGone      = errors.New("contract is no longer active")
	ErrLedgerUnavailable = errors.New("ledger unavailable")

	ErrAmbiguousParty = errors.New("party hint is ambiguous")
	ErrTopologyFrozen = errors.New("the network is being upgraded and does not accept new parties right now")

	ErrSignatureRejected = errors.New("signature does not match this action's party")

	ErrFaucetUnavailable = errors.New("the faucet is not available on this network")
	ErrFaucetCooldown    = errors.New("the faucet was used recently")
)

func Errorf(kind error, format string, args ...any) error {
	return fmt.Errorf("%w: %w", kind, fmt.Errorf(format, args...))
}

func Invalidf(format string, args ...any) error {
	return Errorf(ErrValidation, format, args...)
}

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
