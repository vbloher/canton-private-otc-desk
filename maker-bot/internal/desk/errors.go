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
	message := strings.TrimSpace(string(body))

	var reply struct {
		Error string `json:"error"`
	}

	if json.Unmarshal(body, &reply) == nil && reply.Error != "" {
		message = reply.Error
	}

	switch {
	case status == http.StatusUnauthorized:
		return fmt.Errorf("%w: %s", ErrUnauthorized, message)
	case status == http.StatusNotFound:
		return fmt.Errorf("%w: %s", ErrNotFound, message)
	case status == http.StatusConflict:
		return fmt.Errorf("%w: %s", ErrConflict, message)
	case status == http.StatusGone:
		return fmt.Errorf("%w: %s", ErrGone, message)
	case status == http.StatusTooManyRequests:
		return fmt.Errorf("%w: %s", ErrTooManyRequests, message)
	case status >= http.StatusInternalServerError:
		return fmt.Errorf("%w: %d: %s", ErrUnavailable, status, message)
	default:
		return fmt.Errorf("desk answered %d: %s", status, message)
	}
}
