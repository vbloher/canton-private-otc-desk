package dto

import "github.com/vbloher/canton-private-otc-desk/services/trading/internal/domain"

// RFQWithQuotes is a taker's RFQ with the quotes the taker may see.
type RFQWithQuotes struct {
	RFQ    domain.RFQ
	Quotes []domain.Quote
}
