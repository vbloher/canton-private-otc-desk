package bot_test

import (
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/maker-bot/internal/desk"
)

func Test_Refresh_QuotesWhatHasNoLiveQuote(t *testing.T) {
	fresh, live, expired, failed := openRFQ(t, "CBTC", "USDCx", true, "0.01", time.Hour),
		openRFQ(t, "CBTC", "USDCx", true, "0.01", time.Hour),
		openRFQ(t, "CBTC", "USDCx", true, "0.01", time.Hour),
		openRFQ(t, "CBTC", "USDCx", true, "0.01", time.Hour)
	closed := openRFQ(t, "CBTC", "USDCx", true, "0.01", time.Hour)
	closed.Open = false

	d := &fakeDesk{balances: funded(t), incoming: []desk.IncomingRFQ{
		{RFQ: fresh}, {RFQ: live, MyQuote: quoted("open")}, {RFQ: expired, MyQuote: quoted("expired")},
		{RFQ: failed, MyQuote: quoted("failed")}, {RFQ: closed},
	}}
	b, _, _ := newBot(t, d)

	b.Event(t.Context(), event(t, "rfq.created", map[string]string{"rfqId": fresh.ID.String()}))

	require.ElementsMatch(t, []uuid.UUID{fresh.ID, expired.ID}, submitted(d))
}

func Test_Connected_RetriesFailedQuotes(t *testing.T) {
	fresh, expired, failed := openRFQ(t, "CBTC", "USDCx", true, "0.01", time.Hour),
		openRFQ(t, "CBTC", "USDCx", true, "0.01", time.Hour),
		openRFQ(t, "CBTC", "USDCx", true, "0.01", time.Hour)

	d := &fakeDesk{balances: funded(t), incoming: []desk.IncomingRFQ{
		{RFQ: fresh}, {RFQ: expired, MyQuote: quoted("expired")}, {RFQ: failed, MyQuote: quoted("failed")},
	}}
	b, _, _ := newBot(t, d)

	b.Connected(t.Context())

	require.ElementsMatch(t, []uuid.UUID{fresh.ID, expired.ID, failed.ID}, submitted(d))
}

func Test_Quote_PriceAndTTL(t *testing.T) {
	long := openRFQ(t, "CBTC", "USDCx", true, "0.01", 30*time.Minute)
	short := openRFQ(t, "CBTC", "USDCx", true, "0.01", 30*time.Second)

	d := &fakeDesk{balances: funded(t), incoming: []desk.IncomingRFQ{{RFQ: long}, {RFQ: short}}}
	b, _, _ := newBot(t, d)

	b.Connected(t.Context())

	require.Equal(t, []submission{
		{RFQID: long.ID, Price: "100150", TTL: 600 * time.Second},
		{RFQID: short.ID, Price: "100150", TTL: 30 * time.Second},
	}, d.submissions)
}

func Test_Quote_SkipsWhenLessThanTenSecondsLeft(t *testing.T) {
	d := &fakeDesk{balances: funded(t), incoming: []desk.IncomingRFQ{
		{RFQ: openRFQ(t, "CBTC", "USDCx", true, "0.01", 9*time.Second)},
	}}
	b, _, _ := newBot(t, d)

	b.Connected(t.Context())

	require.Empty(t, d.submissions)
}

func Test_Quote_SkipsAnUnknownSymbol(t *testing.T) {
	unknown := openRFQ(t, "ETHx", "USDCx", true, "1", time.Hour)
	known := openRFQ(t, "CBTC", "USDCx", true, "0.01", time.Hour)

	d := &fakeDesk{balances: funded(t), incoming: []desk.IncomingRFQ{{RFQ: unknown}, {RFQ: known}}}
	b, _, out := newBot(t, d)

	b.Connected(t.Context())

	require.Equal(t, []uuid.UUID{known.ID}, submitted(d), "the next RFQ is still quoted")
	require.Equal(t, 1, out.count(t, "INFO", "rfq skipped"))
}

func Test_Quote_BalanceGate(t *testing.T) {
	sell := openRFQ(t, "CBTC", "USDCx", false, "0.01", time.Hour)

	poor := &fakeDesk{
		balances: map[string]*big.Rat{"CBTC": rat(t, "0"), "USDCx": rat(t, "1000")},
		incoming: []desk.IncomingRFQ{{RFQ: sell}},
	}
	b, _, _ := newBot(t, poor)
	b.Connected(t.Context())
	require.Empty(t, poor.submissions, "999.4985 USDCx is more than half of 1000")

	rich := &fakeDesk{
		balances: map[string]*big.Rat{"USDCx": rat(t, "2000")},
		incoming: []desk.IncomingRFQ{{RFQ: sell}},
	}
	b, _, _ = newBot(t, rich)
	b.Connected(t.Context())
	require.Equal(t, []uuid.UUID{sell.ID}, submitted(rich))
}

func Test_Quote_ConflictIsNotRetried(t *testing.T) {
	rfq := openRFQ(t, "CBTC", "USDCx", true, "0.01", time.Hour)

	d := &fakeDesk{
		balances:   funded(t),
		incoming:   []desk.IncomingRFQ{{RFQ: rfq}},
		submitErrs: []error{fmt.Errorf("submit quote: %w", desk.ErrConflict)},
	}
	b, _, out := newBot(t, d)

	b.Connected(t.Context())

	d.mu.Lock()
	d.incoming = []desk.IncomingRFQ{{RFQ: rfq, MyQuote: quoted("open")}}
	d.mu.Unlock()

	b.Event(t.Context(), event(t, "rfq.created", map[string]string{"rfqId": uuid.NewString()}))

	require.Len(t, d.submissions, 1)
	require.Zero(t, out.count(t, "WARN", ""), "a quote the desk already has is not a failure")
}
