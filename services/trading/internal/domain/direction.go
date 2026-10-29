package domain

type Direction string

const (
	Buy  Direction = "BUY"
	Sell Direction = "SELL"
)

func ParseDirection(s string) (Direction, error) {
	switch d := Direction(s); d {
	case Buy, Sell:
		return d, nil
	default:
		return "", Errorf(ErrValidation, "direction must be BUY or SELL, got %q", s)
	}
}

func (d Direction) TakerBuys() bool {
	return d == Buy
}
