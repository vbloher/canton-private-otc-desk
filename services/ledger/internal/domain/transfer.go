package domain

import "github.com/vbloher/canton-private-otc-desk/pkg/decimal"

const FaucetHint = "faucet"

type IncomingTransfer struct {
	ContractID   string
	Sender       string
	Admin        string
	InstrumentID string
	Symbol       string
	Amount       decimal.Decimal
}

func (t IncomingTransfer) SenderHint() string {
	if t.Sender == "" {
		return FaucetHint
	}

	return Hint(t.Sender)
}
