package dto

import "github.com/vbloher/canton-private-otc-desk/services/ledger/internal/domain"

type ListIncomingInput struct {
	PartyID string
}

type ListIncomingOutput struct {
	Transfers []domain.IncomingTransfer
}
