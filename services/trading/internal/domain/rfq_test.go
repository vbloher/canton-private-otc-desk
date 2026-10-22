package domain_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/pkg/decimal"
	"github.com/vbloher/canton-private-otc-desk/services/trading/internal/domain"
)

var desk = domain.Desk{Operator: operator, Instruments: []string{"CBTC", "USDCx", "CETH"}}

func validParams() domain.RFQParams {
	return domain.RFQParams{
		Taker:     domain.Party{ID: taker, Hint: "alice"},
		Direction: domain.Buy,
		Qty:       decimal.MustParse("0.5"),
		Base:      "CBTC",
		Quote:     "USDCx",
		Makers:    []domain.Party{{ID: makerA, Hint: "makerA"}, {ID: makerB, Hint: "makerB"}},
		TTL:       5 * time.Minute,
		FeeBps:    10,
	}
}

func Test_NewRFQ_Valid(t *testing.T) {
	now := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)

	rfq, err := domain.NewRFQ(validParams(), desk, now)
	require.NoError(t, err)
	require.NotZero(t, rfq.ID)
	require.Equal(t, taker, rfq.TakerParty)
	require.Equal(t, "alice", rfq.TakerHint)
	require.Equal(t, domain.Buy, rfq.Direction)
	require.Equal(t, "0.5", rfq.Qty.String())
	require.Equal(t, []domain.Party{{ID: makerA, Hint: "makerA"}, {ID: makerB, Hint: "makerB"}}, rfq.Makers)
	require.Equal(t, int64(10), rfq.FeeBps)
	require.Equal(t, domain.RFQOpen, rfq.Status)
	require.Equal(t, now, rfq.CreatedAt)
	require.Equal(t, now.Add(5*time.Minute), rfq.Deadline)
	require.Nil(t, rfq.ClosedAt)
	require.True(t, rfq.HasMaker(makerB))
	require.False(t, rfq.HasMaker(taker))
}

func Test_NewRFQ_Validation(t *testing.T) {
	cases := []struct {
		name   string
		modify func(p *domain.RFQParams)
	}{
		{"same base and quote", func(p *domain.RFQParams) { p.Quote = "CBTC" }},
		{"ttl 29", func(p *domain.RFQParams) { p.TTL = 29 * time.Second }},
		{"ttl above an hour", func(p *domain.RFQParams) { p.TTL = time.Hour + time.Second }},
		{"taker in makers", func(p *domain.RFQParams) {
			p.Makers = append(p.Makers, domain.Party{ID: taker, Hint: "alice"})
		}},
		{"unknown base", func(p *domain.RFQParams) { p.Base = "DOGE" }},
		{"unknown quote", func(p *domain.RFQParams) { p.Quote = "DOGE" }},
		{"zero qty", func(p *domain.RFQParams) { p.Qty = decimal.Zero }},
		{"unknown direction", func(p *domain.RFQParams) { p.Direction = "HOLD" }},
		{"no makers", func(p *domain.RFQParams) { p.Makers = nil }},
		{"eleven makers", func(p *domain.RFQParams) {
			p.Makers = make([]domain.Party, 11)
			for i := range p.Makers {
				p.Makers[i] = domain.Party{ID: "maker::" + string(rune('a'+i)), Hint: string(rune('a' + i))}
			}
		}},
		{"duplicate makers", func(p *domain.RFQParams) { p.Makers = append(p.Makers, p.Makers[0]) }},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := validParams()
			c.modify(&p)

			_, err := domain.NewRFQ(p, desk, time.Now())
			require.ErrorIs(t, err, domain.ErrValidation)
			require.True(t, domain.IsInvalidInput(err))
		})
	}
}

func Test_ParseDirection(t *testing.T) {
	d, err := domain.ParseDirection("SELL")
	require.NoError(t, err)
	require.Equal(t, domain.Sell, d)

	_, err = domain.ParseDirection("sell")
	require.ErrorIs(t, err, domain.ErrValidation)
}
