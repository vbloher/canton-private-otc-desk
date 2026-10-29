package domain

import "github.com/vbloher/canton-private-otc-desk/pkg/decimal"

type TransferLeg struct {
	ID         string
	Sender     string
	Receiver   string
	Instrument Instrument
	Amount     decimal.Decimal
}

func NewTransferLeg(id, sender, receiver string, instrument Instrument, amount string) (TransferLeg, error) {
	if id == "" || sender == "" || receiver == "" || instrument.Symbol == "" {
		return TransferLeg{}, Invalidf("leg %q: id, sender, receiver and instrument are required", id)
	}

	if sender == receiver {
		return TransferLeg{}, Invalidf("leg %q: sender and receiver are the same party", id)
	}

	value, err := decimal.Parse(amount)
	if err != nil || !value.IsPositive() {
		return TransferLeg{}, Invalidf("leg %q: amount %q is not a positive decimal", id, amount)
	}

	return TransferLeg{ID: id, Sender: sender, Receiver: receiver, Instrument: instrument, Amount: value}, nil
}
