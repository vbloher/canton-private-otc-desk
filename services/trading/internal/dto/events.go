package dto

import (
	"github.com/google/uuid"

	"github.com/vbloher/canton-private-otc-desk/services/trading/internal/domain"
)

// RFQEvent is the data of rfq.created and rfq.updated.
type RFQEvent struct {
	RFQID uuid.UUID `json:"rfqId"`
}

// QuoteEvent is the data of quote.updated.
type QuoteEvent struct {
	QuoteID uuid.UUID          `json:"quoteId"`
	RFQID   uuid.UUID          `json:"rfqId"`
	Status  domain.QuoteStatus `json:"status"`
}

// TradeEvent is the data of trade.updated.
type TradeEvent struct {
	TradeID uuid.UUID          `json:"tradeId"`
	RFQID   uuid.UUID          `json:"rfqId"`
	Status  domain.TradeStatus `json:"status"`
}
