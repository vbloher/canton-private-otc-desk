package dto

type AllocateExternalPartyInput struct {
	Transactions []string
	Signature    []byte // raw Ed25519 over the topology's multi-hash
	Fingerprint  string
}

type AllocateExternalPartyOutput struct {
	PartyID string
}
