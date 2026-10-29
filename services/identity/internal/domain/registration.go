package domain

import "github.com/google/uuid"

type Topology struct {
	PartyID      string   `json:"party_id"`
	Fingerprint  string   `json:"fingerprint"`
	MultiHash    []byte   `json:"multi_hash"`
	Transactions []string `json:"transactions"`
}

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

func (r Registration) NewUser(partyID, credentialID, encryptedBlob string) (User, error) {
	return NewPasskeyUser(partyID, r.Hint, r.DisplayName, r.PublicKey, credentialID, encryptedBlob)
}

func (r Registration) VerifySignature(signature []byte) error {
	if !VerifySignature(r.PublicKey, r.Topology.MultiHash, signature) {
		return Invalidf("signature does not match the onboarding hash")
	}

	return nil
}
