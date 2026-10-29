package dto

import "github.com/vbloher/canton-private-otc-desk/services/trading/internal/domain"

type RFQWithQuotes struct {
	RFQ    domain.RFQ
	Quotes []domain.Quote
}
