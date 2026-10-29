package domain

import (
	"errors"
	"fmt"
)

var (
	ErrNotFound          = errors.New("not found")
	ErrValidation        = errors.New("validation failed")
	ErrRFQClosed         = errors.New("rfq is closed")
	ErrAlreadyQuoted     = errors.New("rfq already quoted by this maker")
	ErrQuoteNotOpen      = errors.New("quote is not open")
	ErrNotAMaker         = errors.New("not a maker")
	ErrDust              = errors.New("trade too small")
	ErrInsufficientFunds = errors.New("insufficient funds")
	ErrForbidden         = errors.New("forbidden")
	ErrSettlementGone    = errors.New("settlement allocations are gone")
	ErrUpstream          = errors.New("upstream service failed")
)

func IsInvalidInput(err error) bool {
	return errors.Is(err, ErrValidation)
}

func Errorf(sentinel error, format string, args ...any) error {
	return fmt.Errorf("%w: %w", sentinel, fmt.Errorf(format, args...))
}
