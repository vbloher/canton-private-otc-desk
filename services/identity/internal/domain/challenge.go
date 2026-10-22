package domain

import "encoding/base64"

const (
	challengePrefix = "otc-desk:"
	challengeBytes  = 24
)

// NewChallenge is the one-time text a wallet signs to connect.
func NewChallenge() string {
	return challengePrefix + base64.RawURLEncoding.EncodeToString(randomBytes(challengeBytes))
}
