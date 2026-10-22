package domain

import "strings"

// Topology is what an external party's key signs to onboard: MultiHash covers every
// transaction, which go back to the ledger unchanged.
type Topology struct {
	PartyID      string
	Fingerprint  string
	MultiHash    []byte
	Transactions []string
}

// Fingerprint of an external party's signing key: the namespace after "::".
func Fingerprint(partyID string) string {
	_, fingerprint, _ := strings.Cut(partyID, "::")

	return fingerprint
}

// Hint is the readable part of a party id, before "::".
func Hint(partyID string) string {
	hint, _, _ := strings.Cut(partyID, "::")

	return hint
}
