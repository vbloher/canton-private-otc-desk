package http_test

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	nethttp "net/http"

	"github.com/vbloher/canton-private-otc-desk/pkg/decimal"
	"github.com/vbloher/canton-private-otc-desk/pkg/metrics"
	"github.com/vbloher/canton-private-otc-desk/pkg/otel"
	"github.com/vbloher/canton-private-otc-desk/pkg/principal"
	"github.com/vbloher/canton-private-otc-desk/pkg/router"
	"github.com/vbloher/canton-private-otc-desk/services/trading/internal/controller/http"
	"github.com/vbloher/canton-private-otc-desk/services/trading/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/trading/internal/usecase"
	"github.com/vbloher/canton-private-otc-desk/services/trading/internal/usecase/mocks"
)

const Any = mock.Anything

var (
	httpMetrics = metrics.NewHTTPServer()

	alice  = principal.Principal{UserID: uuid.New(), PartyID: "alice::1220", Hint: "alice"}
	makerA = domain.Party{ID: "makerA::1220", Hint: "makerA"}
	desk   = domain.Desk{Operator: "operator::1220", Instruments: []string{"CBTC", "USDCx"}}
)

type deps struct {
	postgres *mocks.Postgres
	ledger   *mocks.Ledger
	identity *mocks.Identity
	redis    *mocks.Redis
	events   *mocks.Events
}

func newServer(t *testing.T) (deps, *httptest.Server) {
	t.Helper()

	otel.SilentModeInit()

	d := deps{
		postgres: mocks.NewPostgres(t),
		ledger:   mocks.NewLedger(t),
		identity: mocks.NewIdentity(t),
		redis:    mocks.NewRedis(t),
		events:   mocks.NewEvents(t),
	}

	tx := mocks.NewTransactor(t)
	tx.EXPECT().Wrap(Any, Any).RunAndReturn(func(ctx context.Context, fn func(context.Context) error) error {
		return fn(ctx)
	}).Maybe()

	uc := usecase.New(usecase.Config{FeeBps: 10, SettleTimeout: 10 * time.Minute},
		tx, d.postgres, d.ledger, d.identity, d.redis, d.events)

	r := router.New()
	require.NoError(t, http.TradingRouter(r, uc, httpMetrics))

	server := httptest.NewServer(r)
	t.Cleanup(server.Close)

	return d, server
}

func call(t *testing.T, server *httptest.Server, method, path, body string, p *principal.Principal) (int, map[string]any) {
	t.Helper()

	req, err := nethttp.NewRequestWithContext(t.Context(), method, server.URL+"/api/v1"+path, strings.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	if p != nil {
		p.SetHeaders(req.Header)
	}

	resp, err := server.Client().Do(req)
	require.NoError(t, err)

	defer resp.Body.Close()

	var out map[string]any
	if resp.ContentLength != 0 && resp.StatusCode != nethttp.StatusNoContent {
		_ = json.NewDecoder(resp.Body).Decode(&out)
	}

	return resp.StatusCode, out
}

const rfqBody = `{"direction":"BUY","qty":"0.5","base":"CBTC","quote":"USDCx","makers":["makerA"],"ttlSeconds":300}`

func Test_Router_RequiresACaller(t *testing.T) {
	_, server := newServer(t)

	status, body := call(t, server, nethttp.MethodGet, "/desk", "", nil)
	require.Equal(t, nethttp.StatusUnauthorized, status)
	require.Equal(t, "unauthorized", body["error"])

	status, body = call(t, server, nethttp.MethodGet, "/desk", "", &alice)
	require.Equal(t, nethttp.StatusOK, status)
	require.InEpsilon(t, 10.0, body["feeBps"], 0)
}

func Test_Router_RejectsOddDecimalsWithTheFieldName(t *testing.T) {
	_, server := newServer(t)

	for _, qty := range []string{"1e5", "-1", "1,5", "0.00000000001"} {
		status, body := call(t, server, nethttp.MethodPost, "/rfqs",
			strings.Replace(rfqBody, `"0.5"`, `"`+qty+`"`, 1), &alice)
		require.Equal(t, nethttp.StatusBadRequest, status, qty)
		require.Contains(t, body["error"], "qty", qty)
	}
}

func Test_Router_CreateRfqSpeaksHints(t *testing.T) {
	d, server := newServer(t)
	d.redis.EXPECT().GetDesk(Any).Return(desk, true)
	d.identity.EXPECT().ResolveMakers(Any, []string{"makerA"}).Return([]domain.Party{makerA}, nil)
	d.postgres.EXPECT().CreateRFQ(Any, Any).Return(nil)
	d.events.EXPECT().Publish(Any, Any, []string{alice.PartyID, makerA.ID}).Return(nil)

	status, body := call(t, server, nethttp.MethodPost, "/rfqs", rfqBody, &alice)
	require.Equal(t, nethttp.StatusCreated, status)
	require.Equal(t, "alice", body["taker"])
	require.Equal(t, []any{"makerA"}, body["makers"])
	require.Equal(t, "0.5", body["qty"])
	require.Equal(t, "open", body["status"])
	require.Equal(t, "BUY", body["direction"])
}

func Test_Router_MapsErrors(t *testing.T) {
	quote := domain.Quote{
		ID: uuid.New(), RFQID: uuid.New(), Maker: makerA, Price: decimal.MustParse("1"),
		ValidUntil: time.Now().Add(time.Minute), Status: domain.QuoteAccepted,
	}
	rfq := domain.RFQ{ID: quote.RFQID, TakerParty: alice.PartyID, Status: domain.RFQOpen}

	t.Run("409 with a code", func(t *testing.T) {
		d, server := newServer(t)
		d.redis.EXPECT().GetDesk(Any).Return(desk, true)
		d.postgres.EXPECT().GetQuote(Any, quote.ID).Return(quote, nil)
		d.postgres.EXPECT().GetRFQForUpdate(Any, rfq.ID).Return(rfq, nil)
		d.postgres.EXPECT().GetQuoteForUpdate(Any, quote.ID).Return(quote, nil)

		status, body := call(t, server, nethttp.MethodPost, "/quotes/"+quote.ID.String()+"/accept", "", &alice)
		require.Equal(t, nethttp.StatusConflict, status)
		require.Equal(t, "QUOTE_NOT_OPEN", body["code"])
		require.Equal(t, "request failed: quote is not open", body["error"])
	})

	t.Run("someone else's rfq is 404", func(t *testing.T) {
		d, server := newServer(t)
		bob := principal.Principal{UserID: uuid.New(), PartyID: "bob::1220", Hint: "bob"}
		d.postgres.EXPECT().GetRFQForUpdate(Any, rfq.ID).Return(rfq, nil)

		status, body := call(t, server, nethttp.MethodDelete, "/rfqs/"+rfq.ID.String(), "", &bob)
		require.Equal(t, nethttp.StatusNotFound, status)
		require.Equal(t, "request failed: not found", body["error"])
	})

	t.Run("ledger down is 502", func(t *testing.T) {
		d, server := newServer(t)
		d.redis.EXPECT().GetDesk(Any).Return(domain.Desk{}, false)
		d.ledger.EXPECT().Desk(Any).Return(domain.Desk{}, domain.Errorf(domain.ErrUpstream, "connection refused"))

		status, body := call(t, server, nethttp.MethodPost, "/rfqs", rfqBody, &alice)
		require.Equal(t, nethttp.StatusBadGateway, status)
		require.NotContains(t, body["error"], "connection refused")
	})
}
