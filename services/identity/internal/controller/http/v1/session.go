package v1

import (
	"time"

	http_server "github.com/vbloher/canton-private-otc-desk/gen/http/identity_v1/server"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/domain"
)

// toSession is the answer of register/complete and login. The token is the secret the cookie carries,
// for a client that cannot use the cookie (a frontend on another origin). The expiry goes out as UTC in
// whole seconds, rounded down so that a client never believes the session outlives the stored expiry.
func toSession(user domain.User, token string, expiresAt time.Time) http_server.Session {
	return http_server.Session{
		Me:           toMe(user),
		SessionToken: token,
		ExpiresAt:    expiresAt.UTC().Truncate(time.Second),
	}
}
