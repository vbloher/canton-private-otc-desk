package domain

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
)

const sessionTokenBytes = 32

func NewSessionToken() (token, hash string) {
	token = base64.RawURLEncoding.EncodeToString(randomBytes(sessionTokenBytes))

	return token, HashSessionToken(token)
}

func HashSessionToken(token string) string {
	return sha256Hex(token)
}

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))

	return hex.EncodeToString(sum[:])
}
