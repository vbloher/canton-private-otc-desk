package ws

import (
	"net/http"
	"strings"

	"github.com/vbloher/canton-private-otc-desk/services/gateway/internal/domain"
)

const (
	// Protocol is the WebSocket subprotocol clients offer and the server accepts.
	Protocol = "otc-desk.v1"

	// bearerPrefix starts the subprotocol entry that carries a session token.
	bearerPrefix = "otc-desk.bearer." //nolint:gosec // G101: a subprotocol prefix, not a credential
)

// BearerToken returns the session token a browser client offers as an "otc-desk.bearer.<token>" subprotocol, or "".
//
// A browser lets a script choose no header of a WebSocket handshake but Sec-WebSocket-Protocol, and a page on
// another origin has no cookie to send, so that is where its session travels. The server never echoes the entry:
// it accepts Protocol only, which is what the browser then sees as the negotiated subprotocol.
//
// The token must be shaped like a bearer token, as in the Authorization header (domain.IsToken68). An entry that
// is not is skipped as if it were not there, and the first well-formed one counts.
func BearerToken(r *http.Request) string {
	for _, line := range r.Header.Values("Sec-WebSocket-Protocol") {
		for offered := range strings.SplitSeq(line, ",") {
			token, ok := strings.CutPrefix(strings.TrimSpace(offered), bearerPrefix)
			if ok && domain.IsToken68(token) {
				return token
			}
		}
	}

	return ""
}
