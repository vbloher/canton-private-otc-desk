package domain

import (
	"slices"

	"github.com/vbloher/canton-private-otc-desk/pkg/decimal"
)

const (
	LegBase  = "base"
	LegQuote = "quote"
	LegFee   = "fee"
)

// Leg is one transfer of a settlement; Instrument is the instrument symbol.
type Leg struct {
	ID         string
	Sender     string
	Receiver   string
	Instrument string
	Amount     decimal.Decimal
}

// Legs are the transfers a trade on the quote settles: base seller → buyer (qty),
// quote buyer → seller (notional = qty × price, floored), and the desk fee maker → operator
// in the quote instrument (notional × fee bps, floored; omitted when it floors to zero).
// A zero base or quote leg is ErrDust.
func Legs(rfq RFQ, quote Quote, operator string) ([]Leg, error) {
	return termsOf(rfq, quote, operator).legs()
}

// Parties lists every sender and receiver of the legs once, in order of appearance.
func Parties(legs []Leg) []string {
	parties := make([]string, 0, len(legs)+1)

	for _, l := range legs {
		for _, p := range []string{l.Sender, l.Receiver} {
			if !slices.Contains(parties, p) {
				parties = append(parties, p)
			}
		}
	}

	return parties
}

// Allocated reports whether every party of the legs has created all of its allocations.
func Allocated(legs []Leg, allocated map[string]bool) bool {
	for _, p := range Parties(legs) {
		if !allocated[p] {
			return false
		}
	}

	return true
}

// NoneAllocated reports whether the ledger answered for every party of the legs, and none of them
// has its allocations: a party missing from the answer is unknown, not unallocated.
func NoneAllocated(legs []Leg, allocated map[string]bool) bool {
	for _, p := range Parties(legs) {
		done, reported := allocated[p]
		if !reported || done {
			return false
		}
	}

	return true
}

// HasFee reports whether the legs carry a desk fee, which the operator must allocate for.
func HasFee(legs []Leg) bool {
	return slices.ContainsFunc(legs, func(l Leg) bool { return l.ID == LegFee })
}

type terms struct {
	direction Direction
	taker     string
	maker     string
	operator  string
	base      string
	quote     string
	qty       decimal.Decimal
	notional  decimal.Decimal
	fee       decimal.Decimal
}

func termsOf(rfq RFQ, quote Quote, operator string) terms {
	notional := rfq.Qty.Mul(quote.Price)

	return terms{
		direction: rfq.Direction,
		taker:     rfq.TakerParty,
		maker:     quote.Maker.ID,
		operator:  operator,
		base:      rfq.Base,
		quote:     rfq.Quote,
		qty:       rfq.Qty,
		notional:  notional,
		fee:       notional.MulBps(rfq.FeeBps),
	}
}

func (t terms) legs() ([]Leg, error) {
	if !t.qty.IsPositive() {
		return nil, Errorf(ErrDust, "its %s leg is zero", LegBase)
	}

	if !t.notional.IsPositive() {
		return nil, Errorf(ErrDust, "its %s leg rounds to zero at 10 decimals", LegQuote)
	}

	buyer, seller := t.taker, t.maker
	if !t.direction.TakerBuys() {
		buyer, seller = t.maker, t.taker
	}

	legs := []Leg{
		{ID: LegBase, Sender: seller, Receiver: buyer, Instrument: t.base, Amount: t.qty},
		{ID: LegQuote, Sender: buyer, Receiver: seller, Instrument: t.quote, Amount: t.notional},
	}

	if t.fee.IsPositive() {
		legs = append(legs, Leg{ID: LegFee, Sender: t.maker, Receiver: t.operator, Instrument: t.quote, Amount: t.fee})
	}

	return legs, nil
}
