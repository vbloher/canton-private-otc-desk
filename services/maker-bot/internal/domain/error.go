package domain

import "errors"

var (
	ErrUnknownInstrument = errors.New("no USD price for instrument")
	ErrInvalidPrice      = errors.New("cannot price")
	ErrInvalidConfig     = errors.New("invalid config")
	ErrConflict          = errors.New("quote refused")
)
