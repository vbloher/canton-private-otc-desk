package domain

import "slices"

// Allocation is a token-standard v2 allocation as its executor sees it.
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

// SlotMatch pairs a slot of the plan with its allocation; an empty ContractID is missing.
type SlotMatch struct {
	Slot

	ContractID string
}

// MatchSlots finds each slot's allocation. Only the plan-matched set belongs to the
// settlement: a stray allocation carrying the same settlement id must never join a batch.
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

// PartyStatus says, per party, whether every one of its slots is allocated.
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
