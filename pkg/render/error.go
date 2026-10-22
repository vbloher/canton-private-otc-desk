package render

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/vbloher/canton-private-otc-desk/pkg/logger"
)

type Err struct {
	Error string `json:"error"`
}

func Error(ctx context.Context, err error, message string) error {
	ctxErr, ok := ctx.Value(logger.ContextErrKey{}).(*error)
	if ok {
		*ctxErr = fmt.Errorf("%s: %w", message, err)
	}

	err = unpack(err)

	return fmt.Errorf("%s: %w", message, err)
}

func InternalErrorJSON(ctx context.Context, w http.ResponseWriter, err error) {
	_ = Error(ctx, err, "internal error")

	JSON(w, Err{Error: http.StatusText(http.StatusInternalServerError)}, http.StatusInternalServerError)
}

func unpack(err error) error {
	for {
		e := errors.Unwrap(err)
		if e == nil {
			break
		}

		err = e
	}

	return err
}
