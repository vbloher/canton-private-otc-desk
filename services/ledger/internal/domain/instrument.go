package domain

import (
	"strconv"
	"strings"

	"github.com/vbloher/canton-private-otc-desk/pkg/decimal"
)

// Instrument is a tradable token. The symbol is the ledger instrument id; the instrument
// admin is the operator.
type Instrument struct {
	Symbol   string
	Name     string
	Decimals int32 // digits the UI shows
}

// InstrumentList is INSTRUMENTS: "SYMBOL:Name:decimals" entries separated by commas.
type InstrumentList []Instrument

func (l *InstrumentList) UnmarshalText(text []byte) error {
	var list InstrumentList

	for entry := range strings.SplitSeq(string(text), ",") {
		parts := strings.Split(entry, ":")
		if len(parts) != 3 {
			return Invalidf("instrument %q: want SYMBOL:Name:decimals", entry)
		}

		symbol, name := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])

		decimals, err := strconv.ParseInt(strings.TrimSpace(parts[2]), 10, 32)
		if err != nil || decimals < 0 || decimals > 10 || symbol == "" || name == "" {
			return Invalidf("instrument %q: want SYMBOL:Name:decimals (0..10)", entry)
		}

		if _, ok := list.Find(symbol); ok {
			return Invalidf("instrument %q listed twice", symbol)
		}

		list = append(list, Instrument{Symbol: symbol, Name: name, Decimals: int32(decimals)})
	}

	*l = list

	return nil
}

func (l InstrumentList) Find(symbol string) (Instrument, bool) {
	for _, inst := range l {
		if inst.Symbol == symbol {
			return inst, true
		}
	}

	return Instrument{}, false
}

// Amounts is FAUCET_AMOUNTS: "SYMBOL:amount" entries separated by commas.
type Amounts map[string]decimal.Decimal

func (a *Amounts) UnmarshalText(text []byte) error {
	amounts := Amounts{}

	for entry := range strings.SplitSeq(string(text), ",") {
		symbol, value, ok := strings.Cut(entry, ":")
		symbol = strings.TrimSpace(symbol)

		amount, err := decimal.Parse(strings.TrimSpace(value))
		if !ok || symbol == "" || err != nil || !amount.IsPositive() {
			return Invalidf("amount %q: want SYMBOL:amount", entry)
		}

		amounts[symbol] = amount
	}

	*a = amounts

	return nil
}
