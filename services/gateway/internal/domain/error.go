package domain

import "errors"

var (
	ErrUnauthorized = errors.New("unauthorized")
	ErrInvalidUser  = errors.New("identity returned an invalid user")
)
