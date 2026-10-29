package domain

import (
	"time"

	"github.com/google/uuid"

	"github.com/vbloher/canton-private-otc-desk/pkg/decimal"
)

type RFQStatus string

const (
	RFQOpen      RFQStatus = "open"
	RFQFilled    RFQStatus = "filled"
	RFQExpired   RFQStatus = "expired"
	RFQCancelled RFQStatus = "cancelled"
)

const (
	MinRFQTTL = 30 * time.Second
	MaxRFQTTL = time.Hour
	MaxMakers = 10
)

type Party struct {
	ID   string
	Hint string
}

type RFQ struct {
	ID         uuid.UUID
	TakerParty string
	TakerHint  string
	Direction  Direction
	Qty        decimal.Decimal
	Base       string
	Quote      string
	Makers     []Party
	FeeBps     int64
	Deadline   time.Time
	Status     RFQStatus
	CreatedAt  time.Time
	ClosedAt   *time.Time
}

type RFQParams struct {
	Taker     Party
	Direction Direction
	Qty       decimal.Decimal
	Base      string
	Quote     string
	Makers    []Party
	TTL       time.Duration
	FeeBps    int64
}

func NewRFQ(p RFQParams, desk Desk, now time.Time) (RFQ, error) {
	err := p.validate(desk)
	if err != nil {
		return RFQ{}, err
	}

	now = now.Truncate(Precision)

	return RFQ{
		ID:         uuid.New(),
		TakerParty: p.Taker.ID,
		TakerHint:  p.Taker.Hint,
		Direction:  p.Direction,
		Qty:        p.Qty,
		Base:       p.Base,
		Quote:      p.Quote,
		Makers:     p.Makers,
		FeeBps:     p.FeeBps,
		Deadline:   now.Add(p.TTL),
		Status:     RFQOpen,
		CreatedAt:  now,
	}, nil
}

func (p RFQParams) validate(desk Desk) error {
	_, err := ParseDirection(string(p.Direction))
	if err != nil {
		return err
	}

	if !p.Qty.IsPositive() {
		return Errorf(ErrValidation, "qty must be positive")
	}

	for _, symbol := range []string{p.Base, p.Quote} {
		if !desk.Knows(symbol) {
			return Errorf(ErrValidation, "unknown instrument %q", symbol)
		}
	}

	if p.Base == p.Quote {
		return Errorf(ErrValidation, "base and quote must differ")
	}

	if p.TTL < MinRFQTTL || p.TTL > MaxRFQTTL {
		return Errorf(ErrValidation, "ttlSeconds must be between %d and %d",
			int(MinRFQTTL.Seconds()), int(MaxRFQTTL.Seconds()))
	}

	return validateMakers(p.Taker, p.Makers)
}

func validateMakers(taker Party, makers []Party) error {
	if len(makers) == 0 || len(makers) > MaxMakers {
		return Errorf(ErrValidation, "between 1 and %d makers required", MaxMakers)
	}

	seen := make(map[string]struct{}, len(makers))

	for _, m := range makers {
		if m.ID == taker.ID {
			return Errorf(ErrValidation, "the taker cannot be one of the makers")
		}

		if _, ok := seen[m.ID]; ok {
			return Errorf(ErrValidation, "maker %q is listed twice", m.Hint)
		}

		seen[m.ID] = struct{}{}
	}

	return nil
}

func (r RFQ) HasMaker(partyID string) bool {
	for _, m := range r.Makers {
		if m.ID == partyID {
			return true
		}
	}

	return false
}

func (r RFQ) Parties() []string {
	parties := make([]string, 0, len(r.Makers)+1)
	parties = append(parties, r.TakerParty)

	for _, m := range r.Makers {
		parties = append(parties, m.ID)
	}

	return parties
}

func (r RFQ) Close(status RFQStatus, at time.Time) RFQ {
	r.Status = status
	r.ClosedAt = &at

	return r
}
