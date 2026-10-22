package dto

import "github.com/vbloher/canton-private-otc-desk/services/trading/internal/domain"

// IncomingRFQ is an RFQ a maker was asked to quote, with the maker's own quote once it exists.
type IncomingRFQ struct {
	RFQ     domain.RFQ
	MyQuote *domain.Quote
}
