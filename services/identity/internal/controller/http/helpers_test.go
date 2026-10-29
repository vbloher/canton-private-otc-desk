package http_test

import (
	"errors"
	"fmt"
)

func failure(sentinel error, detail string) error {
	if sentinel == nil {
		return errors.New(detail)
	}

	return fmt.Errorf("%w: %w", sentinel, errors.New(detail))
}
