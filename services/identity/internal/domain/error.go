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
	ErrOnboardingPaused    = errors.New(
		"the network is being upgraded and cannot create new wallets right now, try again later",
	)
)

func IsInvalidInput(err error) bool {
	return errors.Is(err, ErrValidation)
}

func Invalidf(format string, args ...any) error {
	return fmt.Errorf("%w: %w", ErrValidation, fmt.Errorf(format, args...))
}
