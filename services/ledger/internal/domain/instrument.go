package domain

import (
	"net/url"
	"strconv"
	"strings"

	"github.com/vbloher/canton-private-otc-desk/pkg/decimal"
)

const instrumentFormat = "SYMBOL|Name|decimals|instrumentId|admin|registryURL"

type Instrument struct {
	Symbol   string
	Name     string
	Decimals int32
	ID       string
	Admin    string
	Registry string
}

type InstrumentList []Instrument

func (l *InstrumentList) UnmarshalText(text []byte) error {
	var list InstrumentList

	for entry := range strings.SplitSeq(string(text), ",") {
		inst, err := parseInstrument(entry)
		if err != nil {
			return err
		}

		if _, ok := list.Find(inst.Symbol); ok {
			return Invalidf("instrument %q listed twice", inst.Symbol)
		}

		if _, ok := list.Of(inst.Admin, inst.ID); ok {
			return Invalidf("instrument %s of %s listed twice", inst.ID, inst.Admin)
		}

		list = append(list, inst)
	}

	*l = list

	return nil
}

func parseInstrument(entry string) (Instrument, error) {
	parts := strings.Split(entry, "|")
	if len(parts) != 6 {
		return Instrument{}, Invalidf("instrument %q: want %s", entry, instrumentFormat)
	}

	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
		if parts[i] == "" {
			return Instrument{}, Invalidf("instrument %q: want %s", entry, instrumentFormat)
		}
	}

	decimals, err := strconv.ParseInt(parts[2], 10, 32)
	if err != nil || decimals < 0 || decimals > 10 {
		return Instrument{}, Invalidf("instrument %q: decimals must be 0..10", entry)
	}

	if !strings.Contains(parts[4], "::") {
		return Instrument{}, Invalidf("instrument %q: the admin must be a full party id", entry)
	}

	if !isHTTPURL(parts[5]) {
		return Instrument{}, Invalidf("instrument %q: the registry must be an http(s) URL", entry)
	}

	return Instrument{
		Symbol:   parts[0],
		Name:     parts[1],
		Decimals: int32(decimals),
		ID:       parts[3],
		Admin:    parts[4],
		Registry: strings.TrimRight(parts[5], "/"),
	}, nil
}

func isHTTPURL(raw string) bool {
	u, err := url.Parse(raw)

	return err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Host != ""
}

func (l InstrumentList) Find(symbol string) (Instrument, bool) {
	for _, inst := range l {
		if inst.Symbol == symbol {
			return inst, true
		}
	}

	return Instrument{}, false
}

func (l InstrumentList) Of(admin, id string) (Instrument, bool) {
	for _, inst := range l {
		if inst.Admin == admin && inst.ID == id {
			return inst, true
		}
	}

	return Instrument{}, false
}

func (l InstrumentList) Registry(admin string) (string, bool) {
	for _, inst := range l {
		if inst.Admin == admin {
			return inst.Registry, true
		}
	}

	return "", false
}

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
