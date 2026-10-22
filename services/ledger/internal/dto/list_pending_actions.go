package dto

import "github.com/vbloher/canton-private-otc-desk/services/ledger/internal/domain"

type ListPendingActionsInput struct {
	PartyID string
}

type ListPendingActionsOutput struct {
	Actions []domain.Action
}
