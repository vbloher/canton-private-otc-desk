package usecase_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/pkg/events"
	"github.com/vbloher/canton-private-otc-desk/services/trading/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/trading/internal/dto"
)

func createInput(makers ...string) dto.CreateRFQInput {
	return dto.CreateRFQInput{
		Taker:      taker,
		Direction:  "BUY",
		Qty:        "0.5",
		Base:       "CBTC",
		Quote:      "USDCx",
		Makers:     makers,
		TTLSeconds: 300,
	}
}

func Test_CreateRFQ_PublishesToMakers(t *testing.T) {
	d := newDeps(t)
	d.cachedDesk()
	d.identity.EXPECT().ResolveMakers(Any, []string{"makerA", "makerB"}).
		Return([]domain.Party{makerB, makerA}, nil)

	var saved domain.RFQ

	d.postgres.EXPECT().CreateRFQ(Any, Any).Run(func(_ context.Context, rfq domain.RFQ) { saved = rfq }).Return(nil)

	var published events.Event

	d.events.EXPECT().Publish(Any, Any, parties(taker, makerA, makerB)).
		Run(func(_ context.Context, ev events.Event, _ ...string) { published = ev }).Return(nil)

	rfq, err := d.useCase(t).CreateRFQ(t.Context(), createInput("makerA", "makerB"))
	require.NoError(t, err)
	require.Equal(t, saved.ID, rfq.ID)
	require.Equal(t, taker.ID, rfq.TakerParty)
	require.Equal(t, []domain.Party{makerA, makerB}, rfq.Makers, "makers keep the taker's order")
	require.Equal(t, domain.Buy, rfq.Direction)
	require.Equal(t, "0.5", rfq.Qty.String())
	require.Equal(t, int64(10), rfq.FeeBps)
	require.Equal(t, domain.RFQOpen, rfq.Status)
	require.Equal(t, 5*time.Minute, rfq.Deadline.Sub(rfq.CreatedAt))
	require.Equal(t, events.Event{Type: events.RFQCreated, Data: dto.RFQEvent{RFQID: rfq.ID}}, published)
}

func Test_CreateRFQ_UnknownMaker(t *testing.T) {
	d := newDeps(t)
	d.cachedDesk()
	d.identity.EXPECT().ResolveMakers(Any, []string{"makerA", "makerZ"}).Return([]domain.Party{makerA}, nil)

	_, err := d.useCase(t).CreateRFQ(t.Context(), createInput("makerA", "makerZ"))
	require.ErrorIs(t, err, domain.ErrNotAMaker)
}

func Test_CreateRFQ_ReadsDeskFromLedgerOnCacheMiss(t *testing.T) {
	d := newDeps(t)
	d.redis.EXPECT().GetDesk(Any).Return(domain.Desk{}, false)
	d.ledger.EXPECT().Desk(Any).Return(desk, nil)
	d.redis.EXPECT().SetDesk(Any, desk).Return()
	d.identity.EXPECT().ResolveMakers(Any, []string{"makerA"}).Return([]domain.Party{makerA}, nil)
	d.postgres.EXPECT().CreateRFQ(Any, Any).Return(nil)
	d.events.EXPECT().Publish(Any, Any, parties(taker, makerA)).Return(nil)

	_, err := d.useCase(t).CreateRFQ(t.Context(), createInput("makerA"))
	require.NoError(t, err)
}

func Test_CreateRFQ_LedgerDown(t *testing.T) {
	d := newDeps(t)
	d.redis.EXPECT().GetDesk(Any).Return(domain.Desk{}, false)
	d.ledger.EXPECT().Desk(Any).Return(domain.Desk{}, domain.Errorf(domain.ErrUpstream, "ledger unavailable"))

	_, err := d.useCase(t).CreateRFQ(t.Context(), createInput("makerA"))
	require.ErrorIs(t, err, domain.ErrUpstream)
}

func Test_CreateRFQ_InvalidInputNeedsNoLedger(t *testing.T) {
	cases := []struct {
		name   string
		modify func(in *dto.CreateRFQInput)
	}{
		{"exponent qty", func(in *dto.CreateRFQInput) { in.Qty = "1e5" }},
		{"negative qty", func(in *dto.CreateRFQInput) { in.Qty = "-1" }},
		{"lower-case direction", func(in *dto.CreateRFQInput) { in.Direction = "buy" }},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := newDeps(t)

			input := createInput("makerA")
			c.modify(&input)

			_, err := d.useCase(t).CreateRFQ(t.Context(), input)
			require.ErrorIs(t, err, domain.ErrValidation)
		})
	}
}
