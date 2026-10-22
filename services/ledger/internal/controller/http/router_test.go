package http_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	nethttp "net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/pkg/decimal"
	"github.com/vbloher/canton-private-otc-desk/pkg/metrics"
	"github.com/vbloher/canton-private-otc-desk/pkg/otel"
	"github.com/vbloher/canton-private-otc-desk/pkg/principal"
	"github.com/vbloher/canton-private-otc-desk/pkg/router"
	controller "github.com/vbloher/canton-private-otc-desk/services/ledger/internal/controller/http"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/usecase"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/usecase/mocks"
)

const (
	Any = mock.Anything

	operator = "operator::1220aa"
	makerA   = "makerA::1220aa"
	alice    = "alice-3f9a1c::1220bb"
)

// The Prometheus collectors register globally, once per process.
var sharedMetrics = sync.OnceValue(metrics.NewHTTPServer) //nolint:gochecknoglobals

type server struct {
	handler  nethttp.Handler
	postgres *mocks.Postgres
	redis    *mocks.Redis
	canton   *mocks.Canton
	events   *mocks.Events
}

// newServer is the wallet API over a use case started against mocks.
func newServer(t *testing.T) server {
	t.Helper()

	otel.SilentModeInit()

	s := server{postgres: mocks.NewPostgres(t), redis: mocks.NewRedis(t), canton: mocks.NewCanton(t), events: mocks.NewEvents(t)}
	redis := s.redis

	tx := mocks.NewTransactor(t)
	tx.EXPECT().Wrap(Any, Any).RunAndReturn(func(ctx context.Context, fn func(context.Context) error) error {
		return fn(ctx)
	}).Maybe()

	rules := domain.Disclosed{ContractID: "rules-cid", CreatedEventBlob: "blob"}

	s.canton.EXPECT().Ready(Any).Return(true).Once()
	s.canton.EXPECT().ResolveParty(Any, "operator").Return(operator, nil).Once()
	s.canton.EXPECT().ResolveParty(Any, "makerA").Return(makerA, nil).Once()
	s.canton.EXPECT().TokenRules(Any, operator).Return(rules, nil).Once()
	s.canton.EXPECT().SynchronizerID(Any, operator).Return("sync", nil).Once()
	redis.EXPECT().SetTokenRules(Any, rules).Once()
	redis.EXPECT().SetSynchronizerID(Any, "sync").Once()

	var instruments domain.InstrumentList
	require.NoError(t, instruments.UnmarshalText([]byte("CBTC:Bitcoin:8,USDCx:USD Coin:2")))

	uc := usecase.New(usecase.Config{
		OperatorHint: "operator", HouseParties: []string{"makerA"}, Instruments: instruments,
		ActionTTL: 10 * time.Minute, StartupInterval: time.Millisecond,
	}, tx, s.postgres, redis, s.canton, s.events)
	require.NoError(t, uc.Init(t.Context()))

	r := router.New()
	require.NoError(t, controller.WalletRouter(r, uc, sharedMetrics()))
	s.handler = r

	return s
}

type response struct {
	status int
	body   string
}

func (s server) call(t *testing.T, method, path, body string, caller *principal.Principal) response {
	t.Helper()

	var reader io.Reader
	if body != "" {
		reader = bytes.NewBufferString(body)
	}

	req := httptest.NewRequestWithContext(t.Context(), method, "/api/v1"+path, reader)
	req.Header.Set("Content-Type", "application/json")

	if caller != nil {
		caller.SetHeaders(req.Header)
	}

	rec := httptest.NewRecorder()
	s.handler.ServeHTTP(rec, req)

	return response{status: rec.Code, body: rec.Body.String()}
}

func as(party string) *principal.Principal {
	return &principal.Principal{UserID: uuid.New(), PartyID: party, Hint: domain.Hint(party)}
}

func errorBody(t *testing.T, r response) (string, *string) {
	t.Helper()

	var body struct {
		Error string  `json:"error"`
		Code  *string `json:"code"`
	}
	require.NoError(t, json.Unmarshal([]byte(r.body), &body), r.body)

	return body.Error, body.Code
}

func Test_NoCaller_Unauthorized(t *testing.T) {
	s := newServer(t)

	r := s.call(t, nethttp.MethodGet, "/wallet/balances", "", nil)
	require.Equal(t, nethttp.StatusUnauthorized, r.status)
}

func Test_ValidationReasonReachesTheBody(t *testing.T) {
	s := newServer(t)

	for reason, body := range map[string]string{
		`amount "0" is not a positive decimal`: `{"symbol":"CBTC","amount":"0","receiver":"makerA::1220aa"}`,
		`unknown instrument: "DOGE"`:           `{"symbol":"DOGE","amount":"1","receiver":"makerA::1220aa"}`,
		`cannot withdraw to yourself`:          `{"symbol":"CBTC","amount":"1","receiver":"alice-3f9a1c::1220bb"}`,
	} {
		r := s.call(t, nethttp.MethodPost, "/wallet/withdraw", body, as(alice))
		require.Equal(t, nethttp.StatusBadRequest, r.status, r.body)

		message, _ := errorBody(t, r)
		require.Contains(t, message, reason)
	}

	r := s.call(t, nethttp.MethodPost, "/tx/"+uuid.NewString()+"/execute", `{"signature":"bm90LTY0LWJ5dGVz"}`, as(alice))
	require.Equal(t, nethttp.StatusBadRequest, r.status, r.body)

	message, _ := errorBody(t, r)
	require.Contains(t, message, "the signature must be 64 bytes, got 12")
}

func Test_Incoming_FaucetSenderAndOnlyTheDesksInstruments(t *testing.T) {
	s := newServer(t)

	s.canton.EXPECT().TransferInstructions(Any, alice).Return([]domain.IncomingTransfer{
		{ContractID: "mint-1", Admin: operator, InstrumentID: "CBTC", Amount: decimal.MustParse("1")},
		{ContractID: "offer-1", Sender: makerA, Admin: operator, InstrumentID: "USDCx", Amount: decimal.MustParse("2.5")},
		{ContractID: "x-1", Sender: makerA, Admin: "someone-else::1220", InstrumentID: "CBTC", Amount: decimal.MustParse("3")},
	}, nil).Once()

	r := s.call(t, nethttp.MethodGet, "/wallet/incoming", "", as(alice))
	require.Equal(t, nethttp.StatusOK, r.status, r.body)
	require.JSONEq(t, `[
		{"contractId":"mint-1","sender":"faucet","symbol":"CBTC","amount":"1.0"},
		{"contractId":"offer-1","sender":"makerA","symbol":"USDCx","amount":"2.5"}
	]`, r.body)
}

func Test_ExecuteAction_GoneContractIsBadGateway(t *testing.T) {
	s := newServer(t)

	signature := base64.StdEncoding.EncodeToString(make([]byte, 64))
	action := domain.NewAction(domain.Intent{PartyID: alice, Purpose: domain.PurposeAllocate, Ref: "s-1",
		Description: "Allocate 1000.0 USDCx for quote s-1"}, domain.Prepared{Hash: []byte("h")}, time.Now(), time.Minute)

	// The signed allocation can never land: the signer must hear it failed, which a 404 or a
	// 409 ("no longer pending") would hide.
	s.postgres.EXPECT().GetActionForUpdate(Any, action.ID).Return(action, nil).Once()
	s.canton.EXPECT().Execute(Any, action.Prepared, alice, make([]byte, 64)).
		Return(fmt.Errorf("%w: %w: LOCAL_VERDICT_INACTIVE_CONTRACTS: inputs consumed", domain.ErrLedger,
			domain.ErrContractGone)).Once()
	s.postgres.EXPECT().UpdateAction(Any, mock.MatchedBy(func(a domain.Action) bool {
		return a.Status == domain.ActionFailed
	})).Return(nil).Once()
	s.events.EXPECT().Publish(Any, Any, Any).Return(nil).Once()

	r := s.call(t, nethttp.MethodPost, "/tx/"+action.ID.String()+"/execute", `{"signature":"`+signature+`"}`, as(alice))
	require.Equal(t, nethttp.StatusBadGateway, r.status, r.body)

	message, _ := errorBody(t, r)
	require.Contains(t, message, "LOCAL_VERDICT_INACTIVE_CONTRACTS")
}

func Test_ExecuteAction_WrongKeyIsBadRequestAndStaysPending(t *testing.T) {
	s := newServer(t)

	signature := base64.StdEncoding.EncodeToString(make([]byte, 64))
	action := domain.NewAction(domain.Intent{PartyID: alice, Purpose: domain.PurposeAllocate, Ref: "s-1"},
		domain.Prepared{Hash: []byte("h")}, time.Now(), time.Minute)

	// No UpdateAction and no event: the action stays pending for its own party's key.
	s.postgres.EXPECT().GetActionForUpdate(Any, action.ID).Return(action, nil).Once()
	s.canton.EXPECT().Execute(Any, action.Prepared, alice, make([]byte, 64)).
		Return(fmt.Errorf("%w: FAILED_TO_EXECUTE_TRANSACTION: Received 0 valid signatures", domain.ErrSignatureRejected)).Once()

	r := s.call(t, nethttp.MethodPost, "/tx/"+action.ID.String()+"/execute", `{"signature":"`+signature+`"}`, as(alice))
	require.Equal(t, nethttp.StatusBadRequest, r.status, r.body)

	message, code := errorBody(t, r)
	require.Contains(t, message, "signature does not match this action's party")
	require.Nil(t, code)
}

func Test_Withdraw_GoneContractIsAConflictToRetry(t *testing.T) {
	s := newServer(t)

	rules := domain.Disclosed{ContractID: "rules-cid", CreatedEventBlob: "blob"}

	// A holding it picked was consumed by another in-flight command: retry with fresh state.
	s.postgres.EXPECT().ListPendingActions(Any, alice, Any).Return(nil, nil).Once()
	s.canton.EXPECT().Holdings(Any, alice).Return([]domain.Holding{{
		ContractID: "h-1", Owner: alice, Admin: operator, InstrumentID: "CBTC", Amount: decimal.MustParse("2"),
	}}, nil).Once()
	s.redis.EXPECT().GetTokenRules(Any).Return(rules, true).Once()
	s.canton.EXPECT().TransferCommand(rules, Any).Return(domain.Command{}, nil).Once()
	s.canton.EXPECT().Prepare(Any, alice, Any, domain.Command{}).
		Return(domain.Prepared{}, fmt.Errorf("%w: %w: CONTRACT_NOT_FOUND", domain.ErrLedger, domain.ErrContractGone)).Once()

	r := s.call(t, nethttp.MethodPost, "/wallet/withdraw", `{"symbol":"CBTC","amount":"1","receiver":"makerA::1220aa"}`, as(alice))
	require.Equal(t, nethttp.StatusConflict, r.status, r.body)

	message, code := errorBody(t, r)
	require.Contains(t, message, "retry")
	require.Nil(t, code, "INSUFFICIENT_FUNDS and ACTION_NOT_PENDING are the only codes")
}

func Test_ExecuteAction_LedgerDownIsBadGateway(t *testing.T) {
	s := newServer(t)

	action := domain.NewAction(domain.Intent{PartyID: alice, Purpose: domain.PurposeAllocate, Ref: "s-1"},
		domain.Prepared{Hash: []byte("h")}, time.Now(), time.Minute)

	s.postgres.EXPECT().GetActionForUpdate(Any, action.ID).Return(action, nil).Once()
	s.canton.EXPECT().Execute(Any, action.Prepared, alice, make([]byte, 64)).
		Return(fmt.Errorf("%w: %w: connection refused", domain.ErrLedger, domain.ErrLedgerUnavailable)).Once()

	signature := base64.StdEncoding.EncodeToString(make([]byte, 64))
	r := s.call(t, nethttp.MethodPost, "/tx/"+action.ID.String()+"/execute", `{"signature":"`+signature+`"}`, as(alice))
	require.Equal(t, nethttp.StatusBadGateway, r.status, r.body)
}
