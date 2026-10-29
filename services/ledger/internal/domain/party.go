package domain

import "strings"

type Topology struct {
	PartyID      string
	Fingerprint  string
	MultiHash    []byte
	Transactions []string
}

func Fingerprint(partyID string) string {
	_, fingerprint, _ := strings.Cut(partyID, "::")

	return fingerprint
}

func Hint(partyID string) string {
	hint, _, _ := strings.Cut(partyID, "::")

	return hint
}
