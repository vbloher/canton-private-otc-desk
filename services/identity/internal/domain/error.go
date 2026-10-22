package domain

import (
	"errors"
	"fmt"
)

var (
	ErrNotFound            = errors.New("not found")
	ErrValidation          = errors.New("validation failed")
	ErrUnauthorized        = errors.New("unauthorized")
	ErrRegistrationExpired = errors.New("registration expired")
	ErrCredentialTaken     = errors.New("credential already registered")
	ErrLedger              = errors.New("ledger failure")
)

func IsInvalidInput(err error) bool {
	return errors.Is(err, ErrValidation)
}

// Invalidf reports what is wrong with the input. The details travel as a second wrapped error,
// so errors.Unwrap stops here and render.Error keeps the whole message for the client.
func Invalidf(format string, args ...any) error {
	return fmt.Errorf("%w: %w", ErrValidation, fmt.Errorf(format, args...))
}
