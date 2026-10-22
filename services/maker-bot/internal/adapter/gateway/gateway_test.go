package gateway_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/pkg/decimal"
	"github.com/vbloher/canton-private-otc-desk/pkg/otel"
	"github.com/vbloher/canton-private-otc-desk/services/maker-bot/internal/adapter/gateway"
	"github.com/vbloher/canton-private-otc-desk/services/maker-bot/internal/domain"
)

func Test_Gateway_ListsIncomingAsTheMaker(t *testing.T) {
	otel.SilentModeInit()

	quoted, fresh := uuid.New(), uuid.New()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/v1/rfqs/incoming", r.URL.Path)
		require.Equal(t, "demo-key-a", r.Header.Get("X-API-Key"))

		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `[
		  {"rfq": {"id": "`+quoted.String()+`", "taker": "alice", "direction": "BUY", "qty": "0.5", "base": "CBTC",
		   "quote": "USDCx", "makers": ["makerA"], "feeBps": 10, "deadline": "2026-10-21T12:05:00Z",
		   "status": "open", "createdAt": "2026-10-21T12:00:00Z"},
		   "myQuote": {"id": "`+uuid.NewString()+`", "rfqId": "`+quoted.String()+`", "maker": "makerA",
		   "price": "100150.0", "validUntil": "2026-10-21T12:01:00Z", "status": "failed",
		   "createdAt": "2026-10-21T12:00:01Z"}},
		  {"rfq": {"id": "`+fresh.String()+`", "taker": "alice", "direction": "SELL", "qty": "1", "base": "CETH",
		   "quote": "USDCx", "makers": ["makerA"], "feeBps": 10, "deadline": "2026-10-21T12:05:00Z",
		   "status": "expired", "createdAt": "2026-10-21T12:00:00Z"}}
		]`)
	}))
	t.Cleanup(server.Close)

	gw, err := gateway.New(gateway.Config{URL: server.URL + "/"})
	require.NoError(t, err)

	rfqs, err := gw.ListIncomingRFQs(t.Context(), "demo-key-a")
	require.NoError(t, err)
	require.Equal(t, []domain.RFQ{
		{ID: quoted, Direction: "BUY", Base: "CBTC", Quote: "USDCx", Open: true, Quoted: true},
		{ID: fresh, Direction: "SELL", Base: "CETH", Quote: "USDCx", Open: false, Quoted: false},
	}, rfqs)
}

func Test_Gateway_SubmitsQuotes(t *testing.T) {
	otel.SilentModeInit()

	rfqID := uuid.New()
	status := http.StatusCreated

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/api/v1/rfqs/"+rfqID.String()+"/quotes", r.URL.Path)
		require.Equal(t, "demo-key-b", r.Header.Get("X-API-Key"))

		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		require.Equal(t, map[string]any{"price": "99600.0", "ttlSeconds": 60.0}, body)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, `{"error": "request failed: rfq is closed", "code": "RFQ_CLOSED"}`)
	}))
	t.Cleanup(server.Close)

	gw, err := gateway.New(gateway.Config{URL: server.URL})
	require.NoError(t, err)

	submit := func() error {
		return gw.SubmitQuote(t.Context(), "demo-key-b", rfqID, decimal.MustParse("99600"), time.Minute)
	}

	require.NoError(t, submit())

	status = http.StatusConflict
	err = submit()
	require.ErrorIs(t, err, domain.ErrConflict)
	require.Contains(t, err.Error(), "RFQ_CLOSED")

	status = http.StatusBadGateway
	require.ErrorIs(t, submit(), gateway.ErrUnexpectedStatus)
}
