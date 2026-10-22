package dto

type SettlementQuery struct {
	SettlementID string
	Legs         []TransferLeg
}

type GetSettlementsInput struct {
	Settlements []SettlementQuery
}

type PartyAllocation struct {
	PartyID   string
	Allocated bool // every allocation the party owes exists
}

type SettlementStatus struct {
	SettlementID string
	Parties      []PartyAllocation
}

type GetSettlementsOutput struct {
	Settlements []SettlementStatus
}
