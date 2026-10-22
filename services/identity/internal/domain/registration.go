package domain

import "github.com/google/uuid"

// Topology is what the ledger prepared for a new external party; the browser key signs MultiHash.
type Topology struct {
	PartyID      string   `json:"party_id"`
	Fingerprint  string   `json:"fingerprint"`
	MultiHash    []byte   `json:"multi_hash"`
	Transactions []string `json:"transactions"`
}

// Registration is a wallet that was started and is waiting for its signature.
type Registration struct {
	ID          uuid.UUID `json:"id"`
	Hint        string    `json:"hint"`
	DisplayName string    `json:"display_name"`
	PublicKey   []byte    `json:"public_key"`
	Topology    Topology  `json:"topology"`
}

func NewRegistration(displayName string, publicKey []byte) (Registration, error) {
	name, err := normalizeDisplayName(displayName)
	if err != nil {
		return Registration{}, err
	}

	err = validatePublicKey(publicKey)
	if err != nil {
		return Registration{}, err
	}

	return Registration{
		ID:          uuid.New(),
		Hint:        NewHint(name),
		DisplayName: name,
		PublicKey:   publicKey,
	}, nil
}

// NewUser turns the finished registration into the wallet's user once the ledger allocated the party.
func (r Registration) NewUser(partyID, credentialID, encryptedBlob string) (User, error) {
	return NewPasskeyUser(partyID, r.Hint, r.DisplayName, r.PublicKey, credentialID, encryptedBlob)
}

// VerifySignature checks the browser key's signature over the onboarding hash.
func (r Registration) VerifySignature(signature []byte) error {
	if !VerifySignature(r.PublicKey, r.Topology.MultiHash, signature) {
		return Invalidf("signature does not match the onboarding hash")
	}

	return nil
}
