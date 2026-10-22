package dto

type CancelSettlementInput struct {
	SettlementID string
}

type CancelSettlementOutput struct {
	Cancelled int
}
