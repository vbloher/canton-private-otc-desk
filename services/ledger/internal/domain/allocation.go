package domain

import "slices"

type Allocation struct {
	ContractID    string
	SettlementID  string
	Authorizer    string
	Admin         string
	InstrumentIDs []string
}

func (a Allocation) fills(settlementID string, slot Slot) bool {
	return a.SettlementID == settlementID &&
		a.Authorizer == slot.Party &&
		a.Admin == slot.Admin &&
		slices.Contains(a.InstrumentIDs, slot.InstrumentID)
}

type SlotMatch struct {
	Slot

	ContractID string
}

func MatchSlots(allocs []Allocation, settlementID string, plan []Slot) []SlotMatch {
	matches := make([]SlotMatch, len(plan))

	for i, slot := range plan {
		matches[i] = SlotMatch{Slot: slot}

		for _, a := range allocs {
			if a.fills(settlementID, slot) {
				matches[i].ContractID = a.ContractID

				break
			}
		}
	}

	return matches
}

func PartyStatus(matches []SlotMatch) map[string]bool {
	status := make(map[string]bool)

	for _, m := range matches {
		if _, seen := status[m.Party]; !seen {
			status[m.Party] = true
		}

		if m.ContractID == "" {
			status[m.Party] = false
		}
	}

	return status
}
