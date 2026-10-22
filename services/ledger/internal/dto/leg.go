package dto

// TransferLeg is a leg as callers send it: parties are full ids, Amount a decimal string.
type TransferLeg struct {
	ID         string
	Sender     string
	Receiver   string
	Instrument string
	Amount     string
}
