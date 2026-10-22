package dto

import "github.com/vbloher/canton-private-otc-desk/services/ledger/internal/domain"

type GetBalancesInput struct {
	PartyID string
}

type GetBalancesOutput struct {
	Balances []domain.Balance
}
