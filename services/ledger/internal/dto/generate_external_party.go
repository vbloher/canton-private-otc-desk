package dto

import "github.com/vbloher/canton-private-otc-desk/services/ledger/internal/domain"

type GenerateExternalPartyInput struct {
	Hint      string
	PublicKey []byte // raw Ed25519, 32 bytes
}

type GenerateExternalPartyOutput struct {
	domain.Topology
}
