package dto

type WithdrawInput struct {
	PartyID  string
	Symbol   string
	Amount   string
	Receiver string // full party id
}
