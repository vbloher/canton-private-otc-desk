package v1

import (
	"context"
	"net/http"
	"strings"
	"unicode"
)

const bearerScheme = "Bearer"

type bearerKey struct{}

// BearerMiddleware puts the token of an "Authorization: Bearer <token>" header into the request
// context, for the handlers that read a session themselves (the strict handlers do not see headers).
// It never rejects a request: a missing or unusable header is simply no token.
func BearerMiddleware(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		token := bearerToken(r.Header)
		if token != "" {
			r = r.WithContext(context.WithValue(r.Context(), bearerKey{}, token))
		}

		next.ServeHTTP(w, r)
	}

	return http.HandlerFunc(fn)
}

// bearerFrom is the bearer token of the request, or "" when it came without a usable one.
func bearerFrom(ctx context.Context) string {
	token, _ := ctx.Value(bearerKey{}).(string)

	return token
}

// bearerToken reads "Bearer <token>" (the scheme in any case, spaces around the token ignored).
// Anything else is no token: no header, a second header, another scheme, no token, or more than one.
func bearerToken(header http.Header) string {
	values := header.Values("Authorization")
	if len(values) != 1 {
		return ""
	}

	scheme, token, found := strings.Cut(strings.TrimSpace(values[0]), " ")
	if !found || !strings.EqualFold(scheme, bearerScheme) {
		return ""
	}

	token = strings.TrimLeft(token, " ")
	if token == "" || strings.ContainsFunc(token, unicode.IsSpace) {
		return ""
	}

	return token
}
