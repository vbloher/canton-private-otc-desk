package presence

import "time"

const (
	TTL = 60 * time.Second

	keyPrefix = "presence:"
)

func Key(partyID string) string {
	return keyPrefix + partyID
}
