package dto

import (
	"github.com/google/uuid"

	"github.com/vbloher/canton-private-otc-desk/services/trading/internal/domain"
)

type RFQEvent struct {
	RFQID uuid.UUID `json:"rfqId"`
}

type QuoteEvent struct {
	QuoteID uuid.UUID          `json:"quoteId"`
	RFQID   uuid.UUID          `json:"rfqId"`
	Status  domain.QuoteStatus `json:"status"`
}

type TradeEvent struct {
	TradeID uuid.UUID          `json:"tradeId"`
	RFQID   uuid.UUID          `json:"rfqId"`
	Status  domain.TradeStatus `json:"status"`
}
