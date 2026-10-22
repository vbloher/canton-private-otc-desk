package http_test

import (
	"errors"
	"fmt"
)

// failure builds an error like the adapters return: it wraps a domain error (when there is one) and
// carries internal detail in its message, which must not reach the client.
func failure(sentinel error, detail string) error {
	if sentinel == nil {
		return errors.New(detail)
	}

	return fmt.Errorf("%w: %w", sentinel, errors.New(detail))
}
