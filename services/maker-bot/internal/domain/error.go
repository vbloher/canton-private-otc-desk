package domain

import "errors"

var (
	ErrUnknownInstrument = errors.New("no USD price for instrument")
	ErrInvalidPrice      = errors.New("cannot price")
	ErrInvalidConfig     = errors.New("invalid config")
	// ErrConflict is the desk refusing a quote with 409 (RFQ closed, already quoted, no funds…).
	ErrConflict = errors.New("quote refused")
)
