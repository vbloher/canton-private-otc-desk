package domain

import (
	"slices"
	"time"

	"github.com/vbloher/canton-private-otc-desk/pkg/decimal"
)

// Holding is a token-standard holding as the Holding interface shows it.
type Holding struct {
	ContractID   string
	Owner        string
	Admin        string
	InstrumentID string
	Amount       decimal.Decimal
	Locked       bool      // funds a pending transfer or allocation
	LockedUntil  time.Time // when the lock lapses; zero for a lock that never does
}

// LockedAt reports whether the holding is locked at now. A lock lapses at its expiry (an
// allocation's settlement deadline, a transfer's execute-before): from then on the ledger
// accepts the holding as an unlocked input again. A lock without an expiry never lapses.
func (h Holding) LockedAt(now time.Time) bool {
	return h.Locked && (h.LockedUntil.IsZero() || now.Before(h.LockedUntil))
}

type Balance struct {
	Symbol    string
	Available decimal.Decimal
	Locked    decimal.Decimal
}

// SelectHoldings picks the owner's holdings of one instrument that are unlocked at now,
// largest first, until they cover need. Visibility is not ownership: the receiver of a
// pending transfer may see the sender's locked holding, so only owned holdings count.
func SelectHoldings(hs []Holding, owner, admin, instrument string, need decimal.Decimal, now time.Time,
) ([]string, error) {
	candidates := make([]Holding, 0, len(hs))

	for _, h := range hs {
		if h.Owner == owner && !h.LockedAt(now) && h.Admin == admin && h.InstrumentID == instrument {
			candidates = append(candidates, h)
		}
	}

	slices.SortStableFunc(candidates, func(a, b Holding) int {
		return b.Amount.Cmp(a.Amount)
	})

	var (
		picked []string
		sum    decimal.Decimal
	)

	for _, h := range candidates {
		if sum.Cmp(need) >= 0 {
			break
		}

		picked = append(picked, h.ContractID)
		sum = sum.Add(h.Amount)
	}

	if sum.Cmp(need) < 0 {
		return nil, InsufficientFundsError{Instrument: instrument, Need: need, Held: sum}
	}

	return picked, nil
}

// Balances sums the owner's holdings of the admin's instruments, in instrument order, zeros
// included; a holding whose lock has lapsed by now is available again.
func Balances(hs []Holding, owner, admin string, instruments []Instrument, now time.Time) []Balance {
	balances := make([]Balance, len(instruments))
	index := make(map[string]int, len(instruments))

	for i, inst := range instruments {
		balances[i] = Balance{Symbol: inst.Symbol}
		index[inst.Symbol] = i
	}

	for _, h := range hs {
		i, ok := index[h.InstrumentID]
		if !ok || h.Owner != owner || h.Admin != admin {
			continue
		}

		if h.LockedAt(now) {
			balances[i].Locked = balances[i].Locked.Add(h.Amount)
		} else {
			balances[i].Available = balances[i].Available.Add(h.Amount)
		}
	}

	return balances
}
