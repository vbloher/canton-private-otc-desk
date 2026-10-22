package dto

import "github.com/vbloher/canton-private-otc-desk/services/ledger/internal/domain"

// ActionsOutput lists the sign actions a call queued; empty for a party the desk hosts,
// whose commands the desk already submitted.
type ActionsOutput struct {
	Actions []domain.Action
}
