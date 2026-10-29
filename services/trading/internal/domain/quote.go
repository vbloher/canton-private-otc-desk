package domain

import (
	"time"

	"github.com/google/uuid"

	"github.com/vbloher/canton-private-otc-desk/pkg/decimal"
)

type QuoteStatus string

const (
	QuoteAllocating QuoteStatus = "allocating"
	QuoteOpen       QuoteStatus = "open"
	QuoteAccepted   QuoteStatus = "accepted"
	QuoteWon        QuoteStatus = "won"
	QuoteLost       QuoteStatus = "lost"
	QuoteExpired    QuoteStatus = "expired"
	QuoteCancelled  QuoteStatus = "cancelled"
	QuoteFailed     QuoteStatus = "failed"
)

const (
	DefaultQuoteTTL = 60 * time.Second
	MinQuoteTTL     = 10 * time.Second
	MaxQuoteTTL     = 600 * time.Second

	SettlementGrace = 30 * time.Minute

	Precision = time.Microsecond
)

type Quote struct {
	ID         uuid.UUID
	RFQID      uuid.UUID
	Maker      Party
	Price      decimal.Decimal
	ValidUntil time.Time
	Status     QuoteStatus
	CreatedAt  time.Time
	OpenedAt   *time.Time
}

func NewQuote(rfq RFQ, maker Party, price decimal.Decimal, ttl time.Duration, now time.Time) (Quote, error) {
	if !rfq.HasMaker(maker.ID) {
		return Quote{}, Errorf(ErrForbidden, "%q is not a maker of rfq %s", maker.Hint, rfq.ID)
	}

	if rfq.Status != RFQOpen || !rfq.Deadline.After(now) {
		return Quote{}, ErrRFQClosed
	}

	if ttl == 0 {
		ttl = DefaultQuoteTTL
	}

	if ttl < MinQuoteTTL || ttl > MaxQuoteTTL {
		return Quote{}, Errorf(ErrValidation, "ttlSeconds must be between %d and %d",
			int(MinQuoteTTL.Seconds()), int(MaxQuoteTTL.Seconds()))
	}

	if !price.IsPositive() {
		return Quote{}, Errorf(ErrValidation, "price must be positive")
	}

	now = now.Truncate(Precision)

	validUntil := now.Add(ttl)
	if validUntil.After(rfq.Deadline) {
		validUntil = rfq.Deadline
	}

	return Quote{
		ID:         uuid.New(),
		RFQID:      rfq.ID,
		Maker:      maker,
		Price:      price,
		ValidUntil: validUntil,
		Status:     QuoteAllocating,
		CreatedAt:  now,
	}, nil
}

func (q Quote) SettlementDeadline() time.Time {
	return q.ValidUntil.Add(SettlementGrace)
}

func (q Quote) Acceptable(now time.Time) bool {
	return q.Status == QuoteOpen && q.ValidUntil.After(now)
}

func (q Quote) Open(at time.Time) Quote {
	at = at.Truncate(Precision)
	q.Status = QuoteOpen
	q.OpenedAt = &at

	return q
}

func (q Quote) VisibleToTaker() bool {
	return q.OpenedAt != nil && q.Status != QuoteFailed
}

func (q Quote) WithStatus(status QuoteStatus) Quote {
	q.Status = status

	return q
}

func (s QuoteStatus) Live() bool {
	return s == QuoteAllocating || s == QuoteOpen
}

func (s QuoteStatus) Dead() bool {
	return s == QuoteExpired || s == QuoteCancelled || s == QuoteLost || s == QuoteFailed
}
