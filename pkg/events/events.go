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
