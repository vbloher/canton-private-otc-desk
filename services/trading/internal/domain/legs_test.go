package domain_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/pkg/decimal"
	"github.com/vbloher/canton-private-otc-desk/services/trading/internal/domain"
)

const (
	taker    = "taker::1220"
	makerA   = "makerA::1220"
	makerB   = "makerB::1220"
	operator = "operator::1220"
)

func rfqOf(direction domain.Direction, qty string, feeBps int64) domain.RFQ {
	return domain.RFQ{
		ID:         uuid.New(),
		TakerParty: taker,
		TakerHint:  "alice",
		Direction:  direction,
		Qty:        decimal.MustParse(qty),
		Base:       "CBTC",
		Quote:      "USDCx",
		Makers:     []domain.Party{{ID: makerA, Hint: "makerA"}},
		FeeBps:     feeBps,
		Status:     domain.RFQOpen,
	}
}

func quoteOf(rfq domain.RFQ, price string) domain.Quote {
	return domain.Quote{
		ID:     uuid.New(),
		RFQID:  rfq.ID,
		Maker:  domain.Party{ID: makerA, Hint: "makerA"},
		Price:  decimal.MustParse(price),
		Status: domain.QuoteOpen,
	}
}

func show(legs []domain.Leg) []string {
	out := make([]string, 0, len(legs))

	for _, l := range legs {
		out = append(out, l.ID+": "+l.Sender+" -> "+l.Receiver+" "+l.Amount.String()+" "+l.Instrument)
	}

	return out
}

func Test_Legs_TakerBuy(t *testing.T) {
	rfq := rfqOf(domain.Buy, "0.5", 10)

	legs, err := domain.Legs(rfq, quoteOf(rfq, "100000"), operator)
	require.NoError(t, err)
	require.Equal(t, []string{
		"base: makerA::1220 -> taker::1220 0.5 CBTC",
		"quote: taker::1220 -> makerA::1220 50000.0 USDCx",
		"fee: makerA::1220 -> operator::1220 50.0 USDCx",
	}, show(legs))
}

func Test_Legs_TakerSell(t *testing.T) {
	rfq := rfqOf(domain.Sell, "0.5", 10)

	legs, err := domain.Legs(rfq, quoteOf(rfq, "100000"), operator)
	require.NoError(t, err)
	require.Equal(t, []string{
		"base: taker::1220 -> makerA::1220 0.5 CBTC",
		"quote: makerA::1220 -> taker::1220 50000.0 USDCx",
		"fee: makerA::1220 -> operator::1220 50.0 USDCx",
	}, show(legs))
}

func Test_Legs_FeeFloorsToZeroIsOmitted(t *testing.T) {
	cases := []struct {
		name   string
		qty    string
		feeBps int64
	}{
		{"fee floors to zero", "0.00000005", 10},
		{"no desk fee", "0.5", 0},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rfq := rfqOf(domain.Buy, c.qty, c.feeBps)

			legs, err := domain.Legs(rfq, quoteOf(rfq, "1"), operator)
			require.NoError(t, err)
			require.Equal(t, []string{
				"base: makerA::1220 -> taker::1220 " + c.qty + " CBTC",
				"quote: taker::1220 -> makerA::1220 " + c.qty + " USDCx",
			}, show(legs))
		})
	}
}

func Test_Legs_DustRejected(t *testing.T) {
	cases := []struct {
		name, qty, price string
	}{
		{"quote leg floors to zero", "0.0000000001", "0.5"},
		{"zero base leg", "0", "100000"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rfq := rfqOf(domain.Buy, c.qty, 10)

			_, err := domain.Legs(rfq, quoteOf(rfq, c.price), operator)
			require.ErrorIs(t, err, domain.ErrDust)
		})
	}
}

func Test_Trade_LegsMatchQuoteLegs(t *testing.T) {
	rfq := rfqOf(domain.Sell, "0.5", 10)
	quote := quoteOf(rfq, "100000")

	expected, err := domain.Legs(rfq, quote, operator)
	require.NoError(t, err)

	trade := domain.NewTrade(rfq, quote, rfq.CreatedAt)
	require.Equal(t, domain.TradeSettling, trade.Status)
	require.Equal(t, "50000.0", trade.Notional.String())
	require.Equal(t, "50.0", trade.Fee.String())

	legs, err := trade.Legs(operator)
	require.NoError(t, err)
	require.Equal(t, show(expected), show(legs))
}

func Test_Allocated_RequiresEveryParty(t *testing.T) {
	rfq := rfqOf(domain.Buy, "0.5", 10)

	legs, err := domain.Legs(rfq, quoteOf(rfq, "100000"), operator)
	require.NoError(t, err)

	require.Equal(t, []string{makerA, taker, operator}, domain.Parties(legs))
	require.True(t, domain.Allocated(legs, map[string]bool{makerA: true, taker: true, operator: true}))
	require.False(t, domain.Allocated(legs, map[string]bool{makerA: true, taker: true}))
	require.False(t, domain.Allocated(legs, map[string]bool{makerA: true, taker: false, operator: true}))
	require.False(t, domain.Allocated(legs, nil))
}

func Test_NoneAllocated_NeedsAnAnswerForEveryParty(t *testing.T) {
	rfq := rfqOf(domain.Buy, "0.5", 10)

	legs, err := domain.Legs(rfq, quoteOf(rfq, "100000"), operator)
	require.NoError(t, err)

	require.True(t, domain.NoneAllocated(legs, map[string]bool{makerA: false, taker: false, operator: false}))
	require.False(t, domain.NoneAllocated(legs, map[string]bool{makerA: false, taker: false, operator: true}))
	require.False(t, domain.NoneAllocated(legs, map[string]bool{makerA: false, taker: false}), "operator not reported")
	require.False(t, domain.NoneAllocated(legs, nil))
}
