package dto

type SettleInput struct {
	SettlementID string
	Legs         []TransferLeg
}

type SettleOutput struct {
	UpdateID string // empty when an earlier attempt already settled
}
