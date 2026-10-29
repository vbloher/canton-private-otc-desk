package dto

import (
	"github.com/google/uuid"

	"github.com/vbloher/canton-private-otc-desk/services/trading/internal/domain"
)

type SubmitQuoteInput struct {
	Maker      domain.Party
	RFQID      uuid.UUID
	Price      string
	TTLSeconds *int
}
