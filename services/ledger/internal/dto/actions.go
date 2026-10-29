package dto

import "github.com/vbloher/canton-private-otc-desk/services/ledger/internal/domain"

type ActionsOutput struct {
	Actions []domain.Action
}
