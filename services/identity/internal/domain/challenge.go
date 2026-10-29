package domain

import "encoding/base64"

const (
	challengePrefix = "otc-desk:"
	challengeBytes  = 24
)

func NewChallenge() string {
	return challengePrefix + base64.RawURLEncoding.EncodeToString(randomBytes(challengeBytes))
}
