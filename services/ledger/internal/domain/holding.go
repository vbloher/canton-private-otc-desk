package domain

import (
	"slices"
	"time"

	"github.com/vbloher/canton-private-otc-desk/pkg/decimal"
)

type Holding struct {
	ContractID   string
	Owner        string
	Admin        string
	InstrumentID string
	Amount       decimal.Decimal
	Locked       bool
	LockedUntil  time.Time
}

func (h Holding) LockedAt(now time.Time) bool {
	return h.Locked && (h.LockedUntil.IsZero() || now.Before(h.LockedUntil))
}

type Balance struct {
	Symbol    string
	Available decimal.Decimal
	Locked    decimal.Decimal
}

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

func Balances(hs []Holding, owner string, instruments []Instrument, now time.Time) []Balance {
	balances := make([]Balance, len(instruments))
	index := make(map[string]int, len(instruments))

	for i, inst := range instruments {
		balances[i] = Balance{Symbol: inst.Symbol}
		index[inst.Admin+"|"+inst.ID] = i
	}

	for _, h := range hs {
		i, ok := index[h.Admin+"|"+h.InstrumentID]
		if !ok || h.Owner != owner {
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
