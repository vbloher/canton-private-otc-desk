package usecase_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/pkg/outbox"
	"github.com/vbloher/canton-private-otc-desk/pkg/tradeevents"
	"github.com/vbloher/canton-private-otc-desk/services/trading/internal/domain"
)

var errOutbox = errors.New("no room in the outbox")

type inTransaction struct{}

type added struct {
	message outbox.Message
	inTx    bool
}

type recordingOutbox struct {
	added []added
	err   error
}

func (o *recordingOutbox) Add(ctx context.Context, m outbox.Message) error {
	if o.err != nil {
		return o.err
	}

	inTx, _ := ctx.Value(inTransaction{}).(bool)
	o.added = append(o.added, added{message: m, inTx: inTx})

	return nil
}

func Test_Tick_AddsTheSettledTradeToTheOutboxInsideTheTransaction(t *testing.T) {
	d := newDeps(t)
	now := time.Now()
	f := newSettledFixture(now)

	var settled domain.Trade

	d.expectAllocated(now, f)
	d.ledger.EXPECT().Settle(Any, f.won.ID, Any).Return("update-1", nil)
	d.expectRecorded(t, f, &settled)

	require.NoError(t, d.useCase(t).Tick(t.Context(), now))

	require.Len(t, d.outbox.added, 1)

	got := d.outbox.added[0]
	require.True(t, got.inTx, "the message must be written with the settlement or not at all")
	require.Equal(t, tradeevents.Topic, got.message.Topic)
	require.Equal(t, tradeevents.TypeSettled, got.message.Type)
	require.Equal(t, f.trade.ID.String(), got.message.Key)
	require.NotEqual(t, uuid.Nil, got.message.ID)

	var event tradeevents.Settled

	require.NoError(t, json.Unmarshal(got.message.Payload, &event))
	require.Equal(t, f.trade.ID, event.TradeID)
	require.Equal(t, "update-1", event.UpdateID)
	require.True(t, event.SettledAt.Equal(now))
	require.Equal(t, taker.ID, event.Taker.PartyID)
	require.Equal(t, "50000.0 USDCx", event.Taker.Gave.Value.String()+" "+event.Taker.Gave.Symbol)
	require.Equal(t, "0.5 CBTC", event.Taker.Received.Value.String()+" "+event.Taker.Received.Symbol)
	require.Equal(t, makerA.ID, event.Maker.PartyID)
	require.Equal(t, "0.5 CBTC", event.Maker.Gave.Value.String()+" "+event.Maker.Gave.Symbol)
	require.Equal(t, "49950.0 USDCx", event.Maker.Received.Value.String()+" "+event.Maker.Received.Symbol)
}

func Test_Tick_AnOutboxFailureAbortsRecordingTheSettlement(t *testing.T) {
	d := newDeps(t)
	now := time.Now()
	f := newSettledFixture(now)

	d.outbox.err = errOutbox
	d.expectAllocated(now, f)
	d.ledger.EXPECT().Settle(Any, f.won.ID, Any).Return("update-1", nil)
	d.postgres.EXPECT().ListQuotes(Any, []uuid.UUID{f.rfq.ID}).Return([]domain.Quote{f.won, f.sibling}, nil)
	d.postgres.EXPECT().UpdateTrade(Any, Any).Return(nil)
	d.postgres.EXPECT().UpdateQuote(Any, quoteIn(f.won.ID, domain.QuoteWon)).Return(nil)
	d.postgres.EXPECT().UpdateRFQ(Any, Any).Return(nil)
	d.postgres.EXPECT().UpdateQuote(Any, quoteIn(f.sibling.ID, domain.QuoteLost)).Return(nil)

	require.ErrorIs(t, d.useCase(t).Tick(t.Context(), now), errOutbox)
}

func Test_Tick_ASettlementRecordedAlreadyAddsNothingToTheOutbox(t *testing.T) {
	d := newDeps(t)
	now := time.Now()
	f := newSettledFixture(now)
	done := f.won.WithStatus(domain.QuoteWon)

	d.expectAnswer(now, f, map[string]bool{taker.ID: true, makerA.ID: true, operator: true})
	d.postgres.EXPECT().GetRFQForUpdate(Any, f.rfq.ID).Return(f.rfq, nil)
	d.postgres.EXPECT().GetQuoteForUpdate(Any, f.won.ID).Return(done, nil)
	d.ledger.EXPECT().Settle(Any, f.won.ID, Any).Return("update-1", nil)

	require.NoError(t, d.useCase(t).Tick(t.Context(), now))
	require.Empty(t, d.outbox.added)
}
