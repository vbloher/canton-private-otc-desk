// Package decimal implements the ledger's Numeric 10 money type: exact, non-negative-capable
// fixed point with ten fraction digits. Every product and quotient floors to ten digits,
// the same rounding Daml applies.
package decimal

import (
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"strings"
)

const scale = 10

var (
	ErrInvalid = errors.New("invalid decimal: expected digits with at most 10 fraction digits")

	pattern = regexp.MustCompile(`^\d{1,28}(\.\d{1,10})?$`)

	one = new(big.Int).Exp(big.NewInt(10), big.NewInt(scale), nil) //nolint:gochecknoglobals
	bps = big.NewInt(10_000)                                       //nolint:gochecknoglobals

	Zero = Decimal{} //nolint:gochecknoglobals
)

// Decimal is a value type; the zero value is 0.
type Decimal struct {
	v *big.Int // scaled by 10^10; nil means zero
}

func Parse(s string) (Decimal, error) {
	if !pattern.MatchString(s) {
		return Decimal{}, fmt.Errorf("%w: %q", ErrInvalid, s)
	}

	whole, frac, _ := strings.Cut(s, ".")
	digits := whole + frac + strings.Repeat("0", scale-len(frac))

	v, ok := new(big.Int).SetString(digits, 10)
	if !ok {
		return Decimal{}, fmt.Errorf("%w: %q", ErrInvalid, s)
	}

	return Decimal{v: v}, nil
}

func MustParse(s string) Decimal {
	d, err := Parse(s)
	if err != nil {
		panic(err)
	}

	return d
}

func (d Decimal) int() *big.Int {
	if d.v == nil {
		return new(big.Int)
	}

	return d.v
}

// String prints the canonical form: no trailing zeros, but always one fraction digit ("5.0").
func (d Decimal) String() string {
	v := d.int()
	sign := ""

	if v.Sign() < 0 {
		sign = "-"
		v = new(big.Int).Neg(v)
	}

	whole, frac := new(big.Int).QuoRem(v, one, new(big.Int))
	fracText := strings.TrimRight(fmt.Sprintf("%0*s", scale, frac.String()), "0")

	if fracText == "" {
		fracText = "0"
	}

	return sign + whole.String() + "." + fracText
}

func (d Decimal) Add(o Decimal) Decimal {
	return Decimal{v: new(big.Int).Add(d.int(), o.int())}
}

func (d Decimal) Sub(o Decimal) Decimal {
	return Decimal{v: new(big.Int).Sub(d.int(), o.int())}
}

// Mul floors the product to ten fraction digits.
func (d Decimal) Mul(o Decimal) Decimal {
	p := new(big.Int).Mul(d.int(), o.int())

	return Decimal{v: p.Quo(p, one)}
}

// MulBps multiplies by basis points (10000 = ×1) and floors.
func (d Decimal) MulBps(n int64) Decimal {
	p := new(big.Int).Mul(d.int(), big.NewInt(n))

	return Decimal{v: p.Quo(p, bps)}
}

// Div floors the quotient to ten fraction digits; it panics on a zero divisor.
func (d Decimal) Div(o Decimal) Decimal {
	p := new(big.Int).Mul(d.int(), one)

	return Decimal{v: p.Quo(p, o.int())}
}

func (d Decimal) Cmp(o Decimal) int {
	return d.int().Cmp(o.int())
}

func (d Decimal) IsZero() bool {
	return d.int().Sign() == 0
}

func (d Decimal) IsPositive() bool {
	return d.int().Sign() > 0
}

func (d Decimal) MarshalText() ([]byte, error) {
	return []byte(d.String()), nil
}

func (d *Decimal) UnmarshalText(text []byte) error {
	parsed, err := Parse(string(text))
	if err != nil {
		return err
	}

	*d = parsed

	return nil
}
