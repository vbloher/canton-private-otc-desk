package domain

import "github.com/vbloher/canton-private-otc-desk/pkg/decimal"

// FaucetHint names the sender of a mint offer, which has no sending party.
const FaucetHint = "faucet"

// IncomingTransfer is a transfer offer waiting for its receiver to accept it. Sender is empty
// for a mint offer (the faucet). Inputs are the sender's holdings it locked: the accept
// consumes them, so they are disclosed to the receiver (a mint offer has none).
type IncomingTransfer struct {
	ContractID   string
	Sender       string
	Admin        string
	InstrumentID string
	Amount       decimal.Decimal
	Inputs       []string
}

func (t IncomingTransfer) SenderHint() string {
	if t.Sender == "" {
		return FaucetHint
	}

	return Hint(t.Sender)
}
