package domain

import (
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
)

type Kind string

const (
	KindPasskey Kind = "passkey"
	KindHouse   Kind = "house"
)

const (
	displayNameMin = 2
	displayNameMax = 32
)

// User is a desk participant: a passkey wallet (an external party whose key lives in the browser)
// or a house maker (a hosted party with an API key).
//
// The json tags describe the copy kept in the session cache; the wrapped key and the API key hash
// are never serialized, so a user read back from the cache lacks them.
type User struct {
	ID            uuid.UUID `json:"id"`
	PartyID       string    `json:"party_id"`
	Hint          string    `json:"hint"`
	DisplayName   string    `json:"display_name"`
	Kind          Kind      `json:"kind"`
	PublicKey     []byte    `json:"public_key"`
	CredentialID  string    `json:"credential_id"`
	EncryptedBlob string    `json:"-"`
	APIKeyHash    string    `json:"-"`
	MakerMode     bool      `json:"maker_mode"`
	CreatedAt     time.Time `json:"created_at"`
}

func NewPasskeyUser(
	partyID, hint, displayName string, publicKey []byte, credentialID, encryptedBlob string,
) (User, error) {
	name, err := normalizeDisplayName(displayName)
	if err != nil {
		return User{}, err
	}

	err = validatePublicKey(publicKey)
	if err != nil {
		return User{}, err
	}

	if partyID == "" || hint == "" {
		return User{}, Invalidf("party id and hint are required")
	}

	err = ValidateCredential(credentialID, encryptedBlob)
	if err != nil {
		return User{}, err
	}

	return User{
		ID:            uuid.New(),
		PartyID:       partyID,
		Hint:          hint,
		DisplayName:   name,
		Kind:          KindPasskey,
		PublicKey:     publicKey,
		CredentialID:  credentialID,
		EncryptedBlob: encryptedBlob,
		CreatedAt:     time.Now().UTC(),
	}, nil
}

// ValidateCredential checks what a wallet needs to sign in again: the credential id of its passkey
// and the key wrapped with it. The registration checks it before the ledger allocates the party.
func ValidateCredential(credentialID, encryptedBlob string) error {
	switch {
	case credentialID == "":
		return Invalidf("credential id is required")
	case encryptedBlob == "":
		return Invalidf("encrypted blob is required")
	}

	return nil
}

// NewHouseUser is a hosted maker the desk signs for; it signs in with its API key and is a maker from the start.
func NewHouseUser(partyID, hint, apiKey string) (User, error) {
	if partyID == "" || hint == "" || apiKey == "" {
		return User{}, Invalidf("house maker needs a party id, a hint and an API key")
	}

	return User{
		ID:          uuid.New(),
		PartyID:     partyID,
		Hint:        hint,
		DisplayName: hint,
		Kind:        KindHouse,
		APIKeyHash:  HashAPIKey(apiKey),
		MakerMode:   true,
		CreatedAt:   time.Now().UTC(),
	}, nil
}

func HashAPIKey(key string) string {
	return sha256Hex(key)
}

func normalizeDisplayName(displayName string) (string, error) {
	name := strings.TrimSpace(displayName)

	length := utf8.RuneCountInString(name)
	if length < displayNameMin || length > displayNameMax {
		return "", Invalidf("display name must be %d..%d characters", displayNameMin, displayNameMax)
	}

	if strings.ContainsFunc(name, unicode.IsControl) {
		return "", Invalidf("display name must not contain control characters")
	}

	return name, nil
}

func validatePublicKey(publicKey []byte) error {
	if len(publicKey) != PublicKeySize {
		return Invalidf("public key must be %d bytes", PublicKeySize)
	}

	return nil
}
