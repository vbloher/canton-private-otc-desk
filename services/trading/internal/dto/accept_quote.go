package dto

import (
	"github.com/google/uuid"

	"github.com/vbloher/canton-private-otc-desk/services/trading/internal/domain"
)

type AcceptQuoteInput struct {
	Taker   domain.Party
	QuoteID uuid.UUID
}
