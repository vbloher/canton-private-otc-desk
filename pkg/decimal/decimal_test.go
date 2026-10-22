package decimal_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/pkg/decimal"
)

func Test_Parse_Accepts(t *testing.T) {
	cases := map[string]string{
		"1":            "1.0",
		"0.1":          "0.1",
		"100000.50":    "100000.5",
		"0.0000000001": "0.0000000001",
		"0":            "0.0",
		"007.70":       "7.7",
	}

	for in, want := range cases {
		d, err := decimal.Parse(in)
		require.NoError(t, err, in)
		require.Equal(t, want, d.String(), in)
	}
}

func Test_Parse_Rejects(t *testing.T) {
	cases := []string{"", "1e5", "-1", "1,5", " 1", "1.", ".5", "0.00000000001", "1" + strings.Repeat("0", 28)}

	for _, in := range cases {
		_, err := decimal.Parse(in)
		require.ErrorIs(t, err, decimal.ErrInvalid, in)
	}
}

func Test_Mul_Floors(t *testing.T) {
	require.Equal(t, "0.9999999999", decimal.MustParse("0.3333333333").Mul(decimal.MustParse("3")).String())
	require.Equal(t, "150000.0", decimal.MustParse("1.5").Mul(decimal.MustParse("100000")).String())
	require.Equal(t, "0.0000000001", decimal.MustParse("0.00001").Mul(decimal.MustParse("0.00001")).String())
	require.Equal(t, "0.0", decimal.MustParse("0.00001").Mul(decimal.MustParse("0.000001")).String())
}

func Test_MulBps(t *testing.T) {
	require.Equal(t, "100.0", decimal.MustParse("100000").MulBps(10).String())
	require.Equal(t, "0.0000000001", decimal.MustParse("0.0000001").MulBps(10).String())
	require.Equal(t, "0.0", decimal.MustParse("0.0000000001").MulBps(10).String())
	require.Equal(t, "100150.0", decimal.MustParse("100000").MulBps(10015).String())
}

func Test_Div_Floors(t *testing.T) {
	require.Equal(t, "0.3333333333", decimal.MustParse("1").Div(decimal.MustParse("3")).String())
	require.Equal(t, "25.0", decimal.MustParse("100000").Div(decimal.MustParse("4000")).String())
}

func Test_AddSubCmp(t *testing.T) {
	a := decimal.MustParse("1.5")
	b := decimal.MustParse("0.25")

	require.Equal(t, "1.75", a.Add(b).String())
	require.Equal(t, "1.25", a.Sub(b).String())
	require.Equal(t, 1, a.Cmp(b))
	require.Equal(t, -1, b.Cmp(a))
	require.Equal(t, 0, a.Cmp(decimal.MustParse("1.50")))
	require.True(t, decimal.Zero.IsZero())
	require.True(t, a.IsPositive())
	require.False(t, decimal.Zero.IsPositive())
}

func Test_Text_RoundTrip(t *testing.T) {
	var d decimal.Decimal

	require.NoError(t, d.UnmarshalText([]byte("42.42")))

	text, err := d.MarshalText()
	require.NoError(t, err)
	require.Equal(t, "42.42", string(text))

	require.ErrorIs(t, d.UnmarshalText([]byte("x")), decimal.ErrInvalid)
}

func Test_ZeroValue_IsZero(t *testing.T) {
	var d decimal.Decimal

	require.True(t, d.IsZero())
	require.Equal(t, "0.0", d.String())
	require.Equal(t, "1.0", d.Add(decimal.MustParse("1")).String())
}
