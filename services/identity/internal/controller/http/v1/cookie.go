package v1

import (
	"fmt"
	"time"
)

const sessionCookieName = "otc_session"

func (h *Handlers) sessionCookie(token string, ttl time.Duration) string {
	return h.cookie(token, int(ttl/time.Second))
}

func (h *Handlers) clearedCookie() string {
	return h.cookie("", 0)
}

func (h *Handlers) cookie(value string, maxAgeSeconds int) string {
	cookie := fmt.Sprintf("%s=%s; Path=/; HttpOnly; SameSite=Lax; Max-Age=%d", sessionCookieName, value, maxAgeSeconds)

	if h.cookieSecure {
		cookie += "; Secure"
	}

	return cookie
}
