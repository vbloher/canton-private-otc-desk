package logger

import (
	"context"
	"fmt"
	"net/http"

	"github.com/rs/zerolog/log"

	"github.com/vbloher/canton-private-otc-desk/pkg/router"
)

type ContextErrKey struct{}

func Middleware(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		var err error

		ctx := context.WithValue(r.Context(), ContextErrKey{}, &err)

		ww := router.WriterWrapper(w)
		next.ServeHTTP(ww, r.WithContext(ctx))

		event := log.Info()

		if err != nil {
			event = log.Error().Err(err)
		}

		event.
			Int("code", ww.Code()).
			Str("method", fmt.Sprintf("%s %s", r.Method, router.ExtractPath(r.Context()))).
			Send()
	}

	return http.HandlerFunc(fn)
}
