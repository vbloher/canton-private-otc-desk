package dto

type AllocateExternalPartyInput struct {
	Transactions []string
	Signature    []byte
	Fingerprint  string
}

type AllocateExternalPartyOutput struct {
	PartyID string
}
