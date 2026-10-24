package domain

import "strings"

// A caller proves who it is with the session cookie identity sets on sign-in (browsers on the gateway's own
// origin), with the same session token as "Authorization: Bearer <token>" (a page on another origin, where the
// cookie would be a third-party one), or with an API key (bots). The API key wins over the session token.
const (
	SessionCookie = "otc_session"
	APIKeyHeader  = "X-API-Key" //nolint:gosec // G101: the name of a header, not a credential
)

// IsToken68 reports whether s is what RFC 7235 allows as a bearer token (token68): letters, digits and -._~+/
// followed by optional = padding. Every way of presenting a session token as a bearer token, the Authorization
// header and the WebSocket subprotocol, goes through this one check, so junk is never taken for a credential and
// never reaches identity. Session tokens are unpadded base64url, well inside it.
func IsToken68(s string) bool {
	body := strings.TrimRight(s, "=")
	if body == "" {
		return false
	}

	for i := range len(body) {
		switch c := body[i]; {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		case strings.IndexByte("-._~+/", c) >= 0:
		default:
			return false
		}
	}

	return true
}
