package domain

import "slices"

// Desk holds the facts the ledger owns: the operator party (executor and fee receiver)
// and the symbols of the tradable instruments.
type Desk struct {
	Operator    string
	Instruments []string
}

func (d Desk) Knows(symbol string) bool {
	return slices.Contains(d.Instruments, symbol)
}
