package tradeevents

import (
	"time"

	"github.com/google/uuid"

	"github.com/vbloher/canton-private-otc-desk/pkg/decimal"
)

const (
	Topic       = "trading.trades.v1"
	TypeSettled = "trade.settled"
)

type Amount struct {
	Symbol string          `json:"symbol"`
	Value  decimal.Decimal `json:"value"`
}

type Side struct {
	PartyID  string `json:"partyId"`
	Gave     Amount `json:"gave"`
	Received Amount `json:"received"`
}

type Settled struct {
	TradeID   uuid.UUID `json:"tradeId"`
	UpdateID  string    `json:"updateId,omitempty"`
	SettledAt time.Time `json:"settledAt"`
	Taker     Side      `json:"taker"`
	Maker     Side      `json:"maker"`
}
