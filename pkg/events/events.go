// Package events is the desk's notification bus: services publish small events to a Redis
// channel per party, the gateway forwards them to that party's WebSocket connections.
// Delivery is at-most-once; clients re-fetch state over REST after every event.
package events

const (
	RFQCreated    = "rfq.created"
	RFQUpdated    = "rfq.updated"
	QuoteUpdated  = "quote.updated"
	TradeUpdated  = "trade.updated"
	SignRequested = "sign.requested"
	SignCompleted = "sign.completed"
	WalletUpdated = "wallet.updated"
)

const channelPrefix = "events:"

type Event struct {
	Type string `json:"type"`
	Data any    `json:"data"`
}

func Channel(partyID string) string {
	return channelPrefix + partyID
}
