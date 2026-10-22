package dto

import "github.com/vbloher/canton-private-otc-desk/services/ledger/internal/domain"

type GetDeskOutput struct {
	OperatorPartyID string
	Instruments     []domain.Instrument
}
