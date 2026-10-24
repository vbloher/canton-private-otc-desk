package http

import (
	"cmp"
	"errors"
	"net/http"
	"strings"

	"github.com/vbloher/canton-private-otc-desk/pkg/principal"
	"github.com/vbloher/canton-private-otc-desk/pkg/render"
	"github.com/vbloher/canton-private-otc-desk/services/gateway/internal/controller/ws"
	"github.com/vbloher/canton-private-otc-desk/services/gateway/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/gateway/internal/usecase"
)

const authorizationHeader = "Authorization"

// credentials reads what a request offers as proof of who it is: a session token, an API key, or both.
// Which of them counts is for the use case to say (the API key wins).
type credentials func(r *http.Request) (sessionToken, apiKey string)

// authenticate resolves the caller from the credentials and puts it into the request context.
// Without a caller the request ends here with 401 (502 when identity cannot answer).
func authenticate(uc *usecase.UseCase, from credentials) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		fn := func(w http.ResponseWriter, r *http.Request) {
			sessionToken, apiKey := from(r)

			p, err := uc.Authenticate(r.Context(), sessionToken, apiKey)
			if err != nil {
				authError(w, r, err)

				return
			}

			next.ServeHTTP(w, r.WithContext(principal.WithContext(r.Context(), p)))
		}

		return http.HandlerFunc(fn)
	}
}

// requestCredentials is X-API-Key for a bot, and for everybody else the session: the bearer token, else the
// cookie. Together with the use case that makes the order X-API-Key, bearer, cookie.
func requestCredentials(r *http.Request) (sessionToken, apiKey string) {
	return cmp.Or(bearerToken(r), sessionCookie(r)), r.Header.Get(domain.APIKeyHeader)
}

// socketCredentials is requestCredentials for the WebSocket upgrade, which also takes the token a browser can
// only offer as a subprotocol. It ranks after the Authorization header, which no browser can set on a socket
// but another kind of client might, and before the cookie.
func socketCredentials(r *http.Request) (sessionToken, apiKey string) {
	return cmp.Or(bearerToken(r), ws.BearerToken(r), sessionCookie(r)), r.Header.Get(domain.APIKeyHeader)
}

func sessionCookie(r *http.Request) string {
	cookie, err := r.Cookie(domain.SessionCookie)
	if err != nil {
		return "" // http.ErrNoCookie: not a browser session
	}

	return cookie.Value
}

// bearerToken is the token of an "Authorization: Bearer <token>" header. Anything else counts as no token at all,
// a malformed header included: it is neither a bad request nor worth asking identity about. So does a request with
// several Authorization headers, as it does for identity, which ends a session by bearer token only in that case.
func bearerToken(r *http.Request) string {
	values := r.Header.Values(authorizationHeader)
	if len(values) != 1 {
		return ""
	}

	scheme, token, found := strings.Cut(values[0], " ")
	if !found || !strings.EqualFold(scheme, "Bearer") {
		return ""
	}

	token = strings.TrimLeft(token, " ")
	if !domain.IsToken68(token) {
		return ""
	}

	return token
}

func authError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, domain.ErrUnauthorized) {
		render.JSON(w, render.Err{Error: "unauthorized"}, http.StatusUnauthorized)

		return
	}

	_ = render.Error(r.Context(), err, "authenticate")

	render.JSON(w, render.Err{Error: "upstream unavailable"}, http.StatusBadGateway)
}
