package domain

import (
	"slices"
	"strings"

	"github.com/vbloher/canton-private-otc-desk/pkg/decimal"
)

// Side names follow the token standard (TransferSide).
type Side string

const (
	SideSender   Side = "SenderSide"
	SideReceiver Side = "ReceiverSide"
)

// LegSide is one party's side of a leg: what it sends to, or receives from, the otherside.
type LegSide struct {
	LegID     string
	Side      Side
	Otherside string
	Amount    decimal.Decimal
}

// Slot is one allocation of a settlement: everything one party sends and receives in one
// instrument of one admin. It funds the sum of its sender sides (Send).
type Slot struct {
	Party        string
	Admin        string
	InstrumentID string
	Sides        []LegSide
	Send         decimal.Decimal
}

func (s Slot) Key() string {
	return s.Party + "|" + s.Admin + "|" + s.InstrumentID
}

// Plan is the allocation plan of a settlement: one slot per (party, admin, instrument),
// sorted by that key in byte order, so the order is the same on every host and every run.
func Plan(legs []TransferLeg, admin string) []Slot {
	slots := make(map[string]*Slot)

	add := func(party, otherside string, leg TransferLeg, side Side) {
		key := Slot{Party: party, Admin: admin, InstrumentID: leg.Instrument}.Key()

		slot, ok := slots[key]
		if !ok {
			slot = &Slot{Party: party, Admin: admin, InstrumentID: leg.Instrument}
			slots[key] = slot
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

// Parties of the plan, in plan order, each once.
func Parties(plan []Slot) []string {
	var out []string

	for _, slot := range plan {
		if !slices.Contains(out, slot.Party) {
			out = append(out, slot.Party)
		}
	}

	return out
}
