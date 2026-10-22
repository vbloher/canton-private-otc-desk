package dto

import (
	"github.com/google/uuid"

	"github.com/vbloher/canton-private-otc-desk/services/trading/internal/domain"
)

type CancelRFQInput struct {
	Taker domain.Party
	RFQID uuid.UUID
}
