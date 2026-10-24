package http

import (
	"net/http"
	"strings"
)

// noStore marks every response under /api as one that no cache, shared or private, may keep. API responses are
// per caller and short-lived, and some carry secrets (a login response holds the session token), so no service
// has to remember to say so. Probes and metrics are not API responses and are left as they are.
//
// It sits outside everything else, so the gateway's own answers (401, 404, 502, preflights) are covered as well as
// the proxied ones; the proxies drop a service's Cache-Control, which would otherwise be added to this one.
func noStore(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api" || strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set(cacheControl, "no-store")
		}

		next.ServeHTTP(w, r)
	})
}
