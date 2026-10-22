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

	// SettlementGrace is how long past ValidUntil every allocation of the settlement stays valid.
	SettlementGrace = 30 * time.Minute

	// Precision is that of Postgres and Daml timestamps: a deadline reads back as it was first used.
	Precision = time.Microsecond
)

// Quote is a maker's price for an RFQ. It is the maker's committed allocation on the
// ledger: it opens once that allocation exists. The quote id is the settlement id.
type Quote struct {
	ID         uuid.UUID
	RFQID      uuid.UUID
	Maker      Party
	Price      decimal.Decimal
	ValidUntil time.Time
	Status     QuoteStatus
	CreatedAt  time.Time
	OpenedAt   *time.Time // when the maker's allocation landed; nil while it never did
}

// NewQuote prices an RFQ for one of its makers; ttl 0 means the default, and the quote
// never outlives the RFQ deadline.
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

// SettlementDeadline is the deadline of every allocation of this quote's settlement.
func (q Quote) SettlementDeadline() time.Time {
	return q.ValidUntil.Add(SettlementGrace)
}

// Acceptable reports whether a taker may accept the quote at now.
func (q Quote) Acceptable(now time.Time) bool {
	return q.Status == QuoteOpen && q.ValidUntil.After(now)
}

// Open returns the quote open from at: the maker's committed allocation is on the ledger.
func (q Quote) Open(at time.Time) Quote {
	at = at.Truncate(Precision)
	q.Status = QuoteOpen
	q.OpenedAt = &at

	return q
}

// VisibleToTaker is true for a quote that once opened, a committed price, unless it failed. A quote
// whose allocation never landed keeps its maker's price private, whatever closed it.
func (q Quote) VisibleToTaker() bool {
	return q.OpenedAt != nil && q.Status != QuoteFailed
}

// WithStatus returns the quote in another status.
func (q Quote) WithStatus(status QuoteStatus) Quote {
	q.Status = status

	return q
}

// Live reports whether the quote may still open or be accepted.
func (s QuoteStatus) Live() bool {
	return s == QuoteAllocating || s == QuoteOpen
}

// Dead reports whether the quote can never trade: the desk releases its allocations. An accepted
// or won quote is not live either, but its allocations belong to its trade.
func (s QuoteStatus) Dead() bool {
	return s == QuoteExpired || s == QuoteCancelled || s == QuoteLost || s == QuoteFailed
}
