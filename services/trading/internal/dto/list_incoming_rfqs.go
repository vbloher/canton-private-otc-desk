package dto

import "github.com/vbloher/canton-private-otc-desk/services/trading/internal/domain"

type IncomingRFQ struct {
	RFQ     domain.RFQ
	MyQuote *domain.Quote
}
