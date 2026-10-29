package domain

import "slices"

type Desk struct {
	Operator    string
	Instruments []string
}

func (d Desk) Knows(symbol string) bool {
	return slices.Contains(d.Instruments, symbol)
}
