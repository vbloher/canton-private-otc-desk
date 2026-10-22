package dto

import "github.com/vbloher/canton-private-otc-desk/services/trading/internal/domain"

type CreateRFQInput struct {
	Taker      domain.Party
	Direction  string
	Qty        string
	Base       string
	Quote      string
	Makers     []string // hints
	TTLSeconds int
}
