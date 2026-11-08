package domain

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"time"
)

const tokenBytes = 32

type Confirmation struct {
	TokenHash   string
	PartyID     string
	Email       string
	ExpiresAt   time.Time
	ConfirmedAt *time.Time
}

func NewConfirmation(
	partyID, email string, now time.Time, ttl time.Duration,
) (confirmation Confirmation, token string) {
	secret := make([]byte, tokenBytes)

	_, _ = rand.Read(secret)

	token = base64.RawURLEncoding.EncodeToString(secret)

	return Confirmation{
		TokenHash: HashToken(token),
		PartyID:   partyID,
		Email:     email,
		ExpiresAt: now.Add(ttl),
	}, token
}

func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))

	return hex.EncodeToString(sum[:])
}

func (c Confirmation) Expired(now time.Time) bool {
	return !now.Before(c.ExpiresAt)
}
