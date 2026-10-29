package domain

import (
	"slices"
	"strings"

	"github.com/vbloher/canton-private-otc-desk/pkg/decimal"
)

type Side string

const (
	SideSender   Side = "SenderSide"
	SideReceiver Side = "ReceiverSide"
)

type LegSide struct {
	LegID     string
	Side      Side
	Otherside string
	Amount    decimal.Decimal
}

type Slot struct {
	Party        string
	Admin        string
	InstrumentID string
	Symbol       string
	Registry     string
	Sides        []LegSide
	Send         decimal.Decimal
}

func (s Slot) Key() string {
	return s.Party + "|" + s.Admin + "|" + s.InstrumentID
}

func Plan(legs []TransferLeg) []Slot {
	slots := make(map[string]*Slot)

	add := func(party, otherside string, leg TransferLeg, side Side) {
		slot := &Slot{
			Party:        party,
			Admin:        leg.Instrument.Admin,
			InstrumentID: leg.Instrument.ID,
			Symbol:       leg.Instrument.Symbol,
			Registry:     leg.Instrument.Registry,
		}

		if existing, ok := slots[slot.Key()]; ok {
			slot = existing
		} else {
			slots[slot.Key()] = slot
		}

		slot.Sides = append(slot.Sides, LegSide{LegID: leg.ID, Side: side, Otherside: otherside, Amount: leg.Amount})

		if side == SideSender {
			slot.Send = slot.Send.Add(leg.Amount)
		}
	}

	for _, leg := range legs {
		add(leg.Sender, leg.Receiver, leg, SideSender)
		add(leg.Receiver, leg.Sender, leg, SideReceiver)
	}

	plan := make([]Slot, 0, len(slots))
	for _, slot := range slots {
		plan = append(plan, *slot)
	}

	slices.SortFunc(plan, func(a, b Slot) int {
		return strings.Compare(a.Key(), b.Key())
	})

	return plan
}

func SlotsOf(plan []Slot, party string) []Slot {
	var out []Slot

	for _, slot := range plan {
		if slot.Party == party {
			out = append(out, slot)
		}
	}

	return out
}

func Parties(plan []Slot) []string {
	var out []string

	for _, slot := range plan {
		if !slices.Contains(out, slot.Party) {
			out = append(out, slot.Party)
		}
	}

	return out
}

type Batch struct {
	Admin          string
	Registry       string
	Legs           []TransferLeg
	AllocationCIDs []string
}

func Batches(legs []TransferLeg, matches []SlotMatch) []Batch {
	var batches []Batch

	index := make(map[string]int)

	for _, leg := range legs {
		i, ok := index[leg.Instrument.Admin]
		if !ok {
			i = len(batches)
			index[leg.Instrument.Admin] = i
			batches = append(batches, Batch{Admin: leg.Instrument.Admin, Registry: leg.Instrument.Registry})
		}

		batches[i].Legs = append(batches[i].Legs, leg)
	}

	for _, m := range matches {
		if i, ok := index[m.Admin]; ok {
			batches[i].AllocationCIDs = append(batches[i].AllocationCIDs, m.ContractID)
		}
	}

	return batches
}
