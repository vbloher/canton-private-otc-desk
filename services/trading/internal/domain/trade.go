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

type Amount struct {
	Instrument string
	Value      decimal.Decimal
}

type Exchange struct {
	Party    string
	Gave     Amount
	Received Amount
}

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

func (t Trade) Exchanges() (taker, maker Exchange) {
	base := Amount{Instrument: t.Base, Value: t.Qty}
	quote := Amount{Instrument: t.Quote, Value: t.Notional}

	if t.Direction.TakerBuys() {
		net := Amount{Instrument: t.Quote, Value: t.Notional.Sub(t.Fee)}

		return Exchange{Party: t.Taker.ID, Gave: quote, Received: base},
			Exchange{Party: t.Maker.ID, Gave: base, Received: net}
	}

	gross := Amount{Instrument: t.Quote, Value: t.Notional.Add(t.Fee)}

	return Exchange{Party: t.Taker.ID, Gave: base, Received: quote},
		Exchange{Party: t.Maker.ID, Gave: gross, Received: base}
}

func (t Trade) Settled(updateID string, at time.Time) Trade {
	t.Status = TradeSettled
	t.UpdateID = updateID
	t.SettledAt = &at

	return t
}

func (t Trade) Failed(reason string) Trade {
	t.Status = TradeFailed
	t.Reason = reason

	return t
}
