package domain

import "errors"

var (
	// ErrUnauthorized means the caller sent no credentials, or identity does not accept them.
	ErrUnauthorized = errors.New("unauthorized")
	// ErrInvalidUser means identity answered with a user the gateway cannot forward upstream.
	ErrInvalidUser = errors.New("identity returned an invalid user")
)
