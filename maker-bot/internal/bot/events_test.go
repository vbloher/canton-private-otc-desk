package bot_test

import (
	"crypto/ed25519"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/maker-bot/internal/desk"
)

func Test_Event_RequotesAfterExpiry(t *testing.T) {
	rfq := openRFQ(t, "CBTC", "USDCx", true, "0.01", time.Hour)
	lapsed := quoted("expired")

	d := &fakeDesk{balances: funded(t), incoming: []desk.IncomingRFQ{{RFQ: rfq, MyQuote: lapsed}}}
	b, _, _ := newBot(t, d)

	b.Event(t.Context(), event(t, "quote.updated", map[string]string{
		"quoteId": lapsed.ID.String(), "rfqId": rfq.ID.String(), "status": "expired",
	}))

	require.Equal(t, []uuid.UUID{rfq.ID}, submitted(d))
}

func Test_Event_RequotesAfterAFailedTrade(t *testing.T) {
	rfq := openRFQ(t, "CBTC", "USDCx", true, "0.01", time.Hour)
	revoked := quoted("failed")
	other := openRFQ(t, "CBTC", "USDCx", true, "0.01", time.Hour)

	d := &fakeDesk{balances: funded(t), incoming: []desk.IncomingRFQ{
		{RFQ: rfq, MyQuote: revoked}, {RFQ: other, MyQuote: quoted("failed")},
	}}
	b, _, _ := newBot(t, d)

	b.Event(t.Context(), event(t, "trade.updated", map[string]string{
		"tradeId": uuid.NewString(), "rfqId": rfq.ID.String(), "status": "failed",
	}))

	require.Equal(t, []uuid.UUID{rfq.ID}, submitted(d), "only the RFQ whose trade failed")
}

func Test_Event_LeavesAFailedAllocation(t *testing.T) {
	rfq := openRFQ(t, "CBTC", "USDCx", true, "0.01", time.Hour)
	failed := quoted("failed")

	d := &fakeDesk{balances: funded(t), incoming: []desk.IncomingRFQ{{RFQ: rfq, MyQuote: failed}}}
	b, _, _ := newBot(t, d)

	b.Event(t.Context(), event(t, "quote.updated", map[string]string{
		"quoteId": failed.ID.String(), "rfqId": rfq.ID.String(), "status": "failed",
	}))

	require.Empty(t, d.submissions)
}

func Test_Event_SignRequestedSigns(t *testing.T) {
	action := desk.Action{ID: uuid.New(), Purpose: "allocate", Description: "Allocate 0.01 CBTC", Hash: []byte("hash")}

	d := &fakeDesk{pending: []desk.Action{action}}
	b, id, _ := newBot(t, d)

	b.Event(t.Context(), event(t, "sign.requested", map[string]string{"actionId": action.ID.String()}))

	require.Len(t, d.executions, 1)
	require.Equal(t, action.ID, d.executions[0].ID)
	require.True(t, ed25519.Verify(id.PublicKey(), []byte("hash"), d.executions[0].Signature))
}

func Test_Event_UnknownTypeIsIgnored(t *testing.T) {
	d := &fakeDesk{}
	b, _, _ := newBot(t, d)

	b.Event(t.Context(), event(t, "something.new", map[string]string{}))

	require.Empty(t, d.callNames())
}
