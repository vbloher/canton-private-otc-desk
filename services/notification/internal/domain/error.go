package domain

import (
	"errors"
	"fmt"
)

var (
	ErrNotFound            = errors.New("not found")
	ErrValidation          = errors.New("validation failed")
	ErrUnauthorized        = errors.New("unauthorized")
	ErrEmailNotConfirmed   = errors.New("confirm your email first")
	ErrConfirmationInvalid = errors.New("this confirmation link is invalid or has expired")
	ErrTooManyRequests     = errors.New("too many confirmation emails requested, try again later")
	ErrEmailDisabled       = errors.New("email is not set up on this desk")
	ErrUndeliverable       = errors.New("the email cannot be delivered")
)

func Invalidf(format string, args ...any) error {
	return fmt.Errorf("%w: %w", ErrValidation, fmt.Errorf(format, args...))
}
