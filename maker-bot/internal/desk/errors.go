package desk

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
)

var (
	ErrUnauthorized    = errors.New("unauthorized")
	ErrNotFound        = errors.New("not found")
	ErrConflict        = errors.New("conflict")
	ErrGone            = errors.New("gone")
	ErrTooManyRequests = errors.New("too many requests")
	ErrUnavailable     = errors.New("desk unavailable")
	ErrNoSigner        = errors.New("no signer to sign in with")
)

func Transient(err error) bool {
	if errors.Is(err, ErrUnavailable) || errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}

	var netErr net.Error

	return errors.As(err, &netErr)
}

func statusError(status int, body []byte) error {
	message, fromDesk := deskMessage(body)

	if status >= http.StatusInternalServerError {
		return fmt.Errorf("%w: %d: %s", ErrUnavailable, status, message)
	}

	if !fromDesk {
		return fmt.Errorf("desk URL answered %d without a desk error: %s", status, message)
	}

	sentinel := sentinelFor(status)
	if sentinel == nil {
		return fmt.Errorf("desk answered %d: %s", status, message)
	}

	return fmt.Errorf("%w: %s", sentinel, message)
}

func deskMessage(body []byte) (string, bool) {
	var reply struct {
		Error string `json:"error"`
	}

	if json.Unmarshal(body, &reply) == nil && reply.Error != "" {
		return reply.Error, true
	}

	return strings.TrimSpace(string(body)), false
}

func sentinelFor(status int) error {
	switch status {
	case http.StatusUnauthorized:
		return ErrUnauthorized
	case http.StatusNotFound:
		return ErrNotFound
	case http.StatusConflict:
		return ErrConflict
	case http.StatusGone:
		return ErrGone
	case http.StatusTooManyRequests:
		return ErrTooManyRequests
	default:
		return nil
	}
}
