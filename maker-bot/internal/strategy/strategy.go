package strategy

import (
	"errors"
	"fmt"
	"math/big"
	"strings"
)

const (
	bpsOne         = 10_000
	fractionDigits = 10
)

var (
	ErrNoPrice     = errors.New("no USD price")
	ErrNotPositive = errors.New("price rounds to zero")
)

func Mid(prices map[string]*big.Rat, base, quote string) (*big.Rat, error) {
	baseUSD, ok := prices[base]
	if !ok {
		return nil, fmt.Errorf("%w for %s", ErrNoPrice, base)
	}

	quoteUSD, ok := prices[quote]
	if !ok || quoteUSD.Sign() <= 0 {
		return nil, fmt.Errorf("%w for %s", ErrNoPrice, quote)
	}

	return new(big.Rat).Quo(baseUSD, quoteUSD), nil
}

func Price(mid *big.Rat, takerBuys bool, spreadBps int64) (*big.Rat, error) {
	factor := bpsOne - spreadBps
	if takerBuys {
		factor = bpsOne + spreadBps
	}

	price := round(new(big.Rat).Mul(mid, big.NewRat(factor, bpsOne)))
	if price.Sign() <= 0 {
		return nil, fmt.Errorf("%w: mid %s", ErrNotPositive, mid.FloatString(fractionDigits))
	}

	return price, nil
}

func Spend(takerBuys bool, qty, price *big.Rat, feeBps int) (base, quote *big.Rat) {
	notional := new(big.Rat).Mul(qty, price)
	fee := new(big.Rat).Mul(notional, big.NewRat(int64(feeBps), bpsOne))

	if takerBuys {
		return new(big.Rat).Set(qty), fee
	}

	return new(big.Rat), notional.Add(notional, fee)
}

func Format(r *big.Rat) string {
	s := r.FloatString(fractionDigits)
	if strings.Contains(s, ".") {
		s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	}

	return s
}

func round(r *big.Rat) *big.Rat {
	rounded, _ := new(big.Rat).SetString(r.FloatString(fractionDigits))

	return rounded
}
