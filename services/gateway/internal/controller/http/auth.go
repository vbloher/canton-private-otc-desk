package http

import (
	"errors"
	"net/http"

	"github.com/vbloher/canton-private-otc-desk/pkg/principal"
	"github.com/vbloher/canton-private-otc-desk/pkg/render"
	"github.com/vbloher/canton-private-otc-desk/services/gateway/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/gateway/internal/usecase"
)

func authenticate(uc *usecase.UseCase) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		fn := func(w http.ResponseWriter, r *http.Request) {
			p, err := uc.Authenticate(r.Context(), sessionToken(r), r.Header.Get(domain.APIKeyHeader))
			if err != nil {
				authError(w, r, err)

				return
			}

			next.ServeHTTP(w, r.WithContext(principal.WithContext(r.Context(), p)))
		}

		return http.HandlerFunc(fn)
	}
}

func sessionToken(r *http.Request) string {
	cookie, err := r.Cookie(domain.SessionCookie)
	if err != nil {
		return ""
	}

	return cookie.Value
}

func authError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, domain.ErrUnauthorized) {
		render.JSON(w, render.Err{Error: "unauthorized"}, http.StatusUnauthorized)

		return
	}

	_ = render.Error(r.Context(), err, "authenticate")

	render.JSON(w, render.Err{Error: "upstream unavailable"}, http.StatusBadGateway)
}
