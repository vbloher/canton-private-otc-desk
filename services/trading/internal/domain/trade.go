package domain

import (
	"time"

	"github.com/google/uuid"

	"github.com/vbloher/canton-private-otc-desk/pkg/decimal"
)

type TradeStatus string

const (
	TradeSettling TradeStatus = "settling"
	TradeSettled  TradeStatus = "settled"
	TradeFailed   TradeStatus = "failed"
)

// Trade is an accepted quote on its way to settlement; its settlement id is QuoteID.
type Trade struct {
	ID        uuid.UUID
	RFQID     uuid.UUID
	QuoteID   uuid.UUID
	Taker     Party
	Maker     Party
	Direction Direction
	Base      string
	Quote     string
	Qty       decimal.Decimal
	Price     decimal.Decimal
	Notional  decimal.Decimal
	Fee       decimal.Decimal
	Status    TradeStatus
	Reason    string
	UpdateID  string
	CreatedAt time.Time
	SettledAt *time.Time
}

// NewTrade is the settling trade an accept of the quote creates; its amounts are final.
func NewTrade(rfq RFQ, quote Quote, now time.Time) Trade {
	t := termsOf(rfq, quote, "")

	return Trade{
		ID:        uuid.New(),
		RFQID:     rfq.ID,
		QuoteID:   quote.ID,
		Taker:     Party{ID: rfq.TakerParty, Hint: rfq.TakerHint},
		Maker:     quote.Maker,
		Direction: rfq.Direction,
		Base:      rfq.Base,
		Quote:     rfq.Quote,
		Qty:       rfq.Qty,
		Price:     quote.Price,
		Notional:  t.notional,
		Fee:       t.fee,
		Status:    TradeSettling,
		CreatedAt: now.Truncate(Precision),
	}
}

// Legs rebuilds the settlement legs from the trade's own amounts.
func (t Trade) Legs(operator string) ([]Leg, error) {
	return terms{
		direction: t.Direction,
		taker:     t.Taker.ID,
		maker:     t.Maker.ID,
		operator:  operator,
		base:      t.Base,
		quote:     t.Quote,
		qty:       t.Qty,
		notional:  t.Notional,
		fee:       t.Fee,
	}.legs()
}

// Settled returns the trade as settled by the ledger update updateID.
func (t Trade) Settled(updateID string, at time.Time) Trade {
	t.Status = TradeSettled
	t.UpdateID = updateID
	t.SettledAt = &at

	return t
}

// Failed returns the trade as failed for the reason.
func (t Trade) Failed(reason string) Trade {
	t.Status = TradeFailed
	t.Reason = reason

	return t
}
