package domain

import (
	"fmt"
	"strings"

	"github.com/vbloher/canton-private-otc-desk/pkg/decimal"
)

const bpsOne = 10_000

func Price(direction, base, quote string, usd map[string]decimal.Decimal, spreadBps int64) (decimal.Decimal, error) {
	baseUSD, ok := usd[base]
	if !ok {
		return decimal.Zero, fmt.Errorf("%w %q", ErrUnknownInstrument, base)
	}

	quoteUSD, ok := usd[quote]
	if !ok || !quoteUSD.IsPositive() {
		return decimal.Zero, fmt.Errorf("%w %q", ErrUnknownInstrument, quote)
	}

	mid := baseUSD.Div(quoteUSD)

	var price decimal.Decimal

	switch direction {
	case "BUY":
		price = mid.MulBps(bpsOne + spreadBps)
	case "SELL":
		price = mid.MulBps(bpsOne - spreadBps)
	default:
		return decimal.Zero, fmt.Errorf("%w: direction %q", ErrInvalidPrice, direction)
	}

	if !price.IsPositive() {
		return decimal.Zero, fmt.Errorf("%w: %s/%s rounds to %s", ErrInvalidPrice, base, quote, price)
	}

	return price, nil
}

type Prices map[string]decimal.Decimal

func (p *Prices) UnmarshalText(text []byte) error {
	prices := Prices{}

	for entry := range strings.SplitSeq(string(text), ",") {
		symbol, value, ok := strings.Cut(strings.TrimSpace(entry), ":")
		if !ok || symbol == "" {
			return fmt.Errorf("%w: price %q is not SYMBOL:USD", ErrInvalidConfig, entry)
		}

		if _, dup := prices[symbol]; dup {
			return fmt.Errorf("%w: %s priced twice", ErrInvalidConfig, symbol)
		}

		usd, err := decimal.Parse(value)
		if err != nil || !usd.IsPositive() {
			return fmt.Errorf("%w: price of %s must be a positive decimal, got %q", ErrInvalidConfig, symbol, value)
		}

		prices[symbol] = usd
	}

	*p = prices

	return nil
}
