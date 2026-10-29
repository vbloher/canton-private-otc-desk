package dto

import "github.com/vbloher/canton-private-otc-desk/services/ledger/internal/domain"

type GenerateExternalPartyInput struct {
	Hint      string
	PublicKey []byte
}

type GenerateExternalPartyOutput struct {
	domain.Topology
}
