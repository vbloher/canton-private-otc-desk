package desk_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/maker-bot/internal/desk"
)

const party = "bot-1a2b3c::1220aa"

var quietLog = slog.New(slog.NewTextHandler(io.Discard, nil))

type fake struct {
	t      *testing.T
	mu     sync.Mutex
	routes map[string]http.HandlerFunc
	logins atomic.Int64
	token  atomic.Value
}

func newFake(t *testing.T, publicKey ed25519.PublicKey) (*fake, *httptest.Server) {
	t.Helper()

	f := &fake{t: t, routes: map[string]http.HandlerFunc{}}
	f.token.Store("")

	f.routes["POST /api/v1/auth/challenge"] = func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		require.Equal(t, map[string]string{"partyId": party}, body)
		reply(w, http.StatusOK, map[string]string{"challenge": "otc-desk:abc", "publicKey": b64(publicKey)})
	}
	f.routes["POST /api/v1/auth/login"] = func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		require.Equal(t, party, body["partyId"])
		require.Equal(t, "otc-desk:abc", body["challenge"])

		signature, err := base64.StdEncoding.DecodeString(body["signature"])
		require.NoError(t, err)
		require.True(t, ed25519.Verify(publicKey, []byte("otc-desk:abc"), signature), "the challenge is signed as UTF-8 text")

		token := "tok-" + strconv.FormatInt(f.logins.Add(1), 10)
		f.token.Store(token)
		http.SetCookie(w, &http.Cookie{Name: "otc_session", Value: token, Path: "/", HttpOnly: true, Secure: true})
		reply(w, http.StatusOK, me())
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		route, ok := f.routes[r.Method+" "+r.URL.Path]
		f.mu.Unlock()

		if !ok {
			reply(w, http.StatusNotFound, map[string]string{"error": "no route " + r.Method + " " + r.URL.Path})

			return
		}

		route(w, r)
	}))
	t.Cleanup(srv.Close)

	return f, srv
}

func (f *fake) route(key string, h http.HandlerFunc) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.routes[key] = h
}

func (f *fake) session(r *http.Request) bool {
	cookie, err := r.Cookie("otc_session")

	return err == nil && cookie.Value != "" && cookie.Value == f.token.Load()
}

func reply(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func me() map[string]any {
	return map[string]any{
		"id": uuid.NewString(), "partyId": party, "hint": "bot-1a2b3c", "displayName": "OTC Demo Maker",
		"kind": "key", "makerMode": true,
	}
}

func b64(b []byte) string {
	return base64.StdEncoding.EncodeToString(b)
}

func newClient(t *testing.T) (*fake, *desk.Client) {
	t.Helper()

	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	f, srv := newFake(t, publicKey)

	c, err := desk.New(srv.URL, srv.Client(), quietLog)
	require.NoError(t, err)
	c.SetSigner(party, func(message []byte) []byte { return ed25519.Sign(privateKey, message) })

	return f, c
}

func (f *fake) incoming(body any) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !f.session(r) {
			reply(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})

			return
		}

		reply(w, http.StatusOK, body)
	}
}

func TestSignIn_SendsPartyIDAndSignature(t *testing.T) {
	f, c := newClient(t)
	f.route("GET /api/v1/rfqs/incoming", f.incoming([]any{}))

	require.NoError(t, c.SignIn(t.Context()))

	_, err := c.IncomingRFQs(t.Context())
	require.NoError(t, err, "the next call carries the session")
	require.Equal(t, int64(1), f.logins.Load())
}

func TestSignIn_IsLogged(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	_, srv := newFake(t, publicKey)

	var out bytes.Buffer

	c, err := desk.New(srv.URL, srv.Client(), slog.New(slog.NewJSONHandler(&out, nil)))
	require.NoError(t, err)
	c.SetSigner(party, func(message []byte) []byte { return ed25519.Sign(privateKey, message) })

	require.NoError(t, c.SignIn(t.Context()))

	var record map[string]any
	require.NoError(t, json.Unmarshal(out.Bytes(), &record))
	require.Equal(t, "signed in", record["msg"])
	require.Equal(t, party, record["party"])
}

func TestSession_SignsInAgainOn401(t *testing.T) {
	f, c := newClient(t)
	f.route("GET /api/v1/rfqs/incoming", f.incoming([]any{}))

	require.NoError(t, c.SignIn(t.Context()))
	f.token.Store("expired-elsewhere")

	_, err := c.IncomingRFQs(t.Context())
	require.NoError(t, err)
	require.Equal(t, int64(2), f.logins.Load(), "one sign-in at start, one after the 401")
}

func TestSession_ConcurrentCallsShareOneSignIn(t *testing.T) {
	f, c := newClient(t)
	f.route("GET /api/v1/rfqs/incoming", f.incoming([]any{}))

	var wg sync.WaitGroup

	for range 8 {
		wg.Go(func() {
			_, err := c.IncomingRFQs(t.Context())
			require.NoError(t, err)
		})
	}

	wg.Wait()
	require.Equal(t, int64(1), f.logins.Load())
}

func TestSession_DoesNotRetryAuthCalls(t *testing.T) {
	f, c := newClient(t)

	var calls atomic.Int64

	f.route("POST /api/v1/auth/login", func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		reply(w, http.StatusUnauthorized, map[string]string{"error": "request failed: unauthorized"})
	})

	err := c.SignIn(t.Context())
	require.ErrorIs(t, err, desk.ErrUnauthorized)
	require.Equal(t, int64(1), calls.Load())
}

func TestSignIn_UnknownParty(t *testing.T) {
	f, c := newClient(t)
	f.route("POST /api/v1/auth/challenge", func(w http.ResponseWriter, _ *http.Request) {
		reply(w, http.StatusNotFound, map[string]string{"error": "request failed: not found"})
	})

	require.ErrorIs(t, c.SignIn(t.Context()), desk.ErrNotFound)
}

func TestSignIn_A404FromSomethingElseIsNotUnknown(t *testing.T) {
	f, c := newClient(t)
	f.route("POST /api/v1/auth/challenge", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("<html>404 page not found</html>"))
	})

	err := c.SignIn(t.Context())
	require.Error(t, err)
	require.NotErrorIs(t, err, desk.ErrNotFound, "only the desk's own answer means the party is unknown")
	require.ErrorContains(t, err, "404")
}

func TestIncomingRFQs_ParsesTheWire(t *testing.T) {
	f, c := newClient(t)

	open, quoted, closed := uuid.New(), uuid.New(), uuid.New()
	deadline := "2026-11-01T12:30:00Z"

	f.route("GET /api/v1/rfqs/incoming", f.incoming([]any{
		map[string]any{
			"rfq": rfq(open, "BUY", "0.01", "open", deadline),
			"myQuote": map[string]any{
				"id": quoted.String(), "rfqId": open.String(), "maker": "bot-1a2b3c", "price": "100150",
				"validUntil": deadline, "status": "expired", "createdAt": deadline,
			},
		},
		map[string]any{"rfq": rfq(closed, "SELL", "2", "filled", deadline)},
	}))
	require.NoError(t, c.SignIn(t.Context()))

	got, err := c.IncomingRFQs(t.Context())
	require.NoError(t, err)
	require.Len(t, got, 2)

	first := got[0]
	require.Equal(t, open, first.RFQ.ID)
	require.True(t, first.RFQ.TakerBuys)
	require.Zero(t, first.RFQ.Qty.Cmp(big.NewRat(1, 100)))
	require.Equal(t, "CBTC", first.RFQ.Base)
	require.Equal(t, "USDCx", first.RFQ.Quote)
	require.Equal(t, 10, first.RFQ.FeeBps)
	require.True(t, first.RFQ.Deadline.Equal(time.Date(2026, 11, 1, 12, 30, 0, 0, time.UTC)))
	require.True(t, first.RFQ.Open)
	require.Equal(t, &desk.Quote{ID: quoted, Status: "expired"}, first.MyQuote)

	require.False(t, got[1].RFQ.TakerBuys)
	require.False(t, got[1].RFQ.Open)
	require.Nil(t, got[1].MyQuote)
}

func rfq(id uuid.UUID, direction, qty, status, deadline string) map[string]any {
	return map[string]any{
		"id": id.String(), "taker": "alice-3f9a1c", "direction": direction, "qty": qty, "base": "CBTC",
		"quote": "USDCx", "makers": []string{"bot-1a2b3c"}, "feeBps": 10, "deadline": deadline, "status": status,
		"createdAt": deadline,
	}
}

func TestSubmitQuote_SendsPriceAndTTL(t *testing.T) {
	f, c := newClient(t)
	id, quoteID := uuid.New(), uuid.New()

	f.route("POST /api/v1/rfqs/"+id.String()+"/quotes", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		require.Equal(t, map[string]any{"price": "100150", "ttlSeconds": float64(600)}, body)
		reply(w, http.StatusCreated, map[string]any{
			"id": quoteID.String(), "rfqId": id.String(), "maker": "bot-1a2b3c", "price": "100150",
			"validUntil": "2026-11-01T12:10:00Z", "status": "allocating", "createdAt": "2026-11-01T12:00:00Z",
		})
	})
	require.NoError(t, c.SignIn(t.Context()))

	q, err := c.SubmitQuote(t.Context(), id, "100150", 600*time.Second)
	require.NoError(t, err)
	require.Equal(t, desk.Quote{ID: quoteID, Status: "allocating"}, q)

	f.route("POST /api/v1/rfqs/"+id.String()+"/quotes", func(w http.ResponseWriter, _ *http.Request) {
		reply(w, http.StatusConflict, map[string]string{"error": "request failed: rfq already quoted by this maker"})
	})

	_, err = c.SubmitQuote(t.Context(), id, "100150", 600*time.Second)
	require.ErrorIs(t, err, desk.ErrConflict)
	require.ErrorContains(t, err, "rfq already quoted by this maker")
}

func TestFaucet_Cooldown(t *testing.T) {
	f, c := newClient(t)
	f.route("POST /api/v1/wallet/faucet", func(w http.ResponseWriter, _ *http.Request) {
		reply(w, http.StatusTooManyRequests, map[string]string{"error": "request failed: faucet cooldown"})
	})
	require.NoError(t, c.SignIn(t.Context()))

	require.ErrorIs(t, c.Faucet(t.Context()), desk.ErrTooManyRequests)
}

func TestWallet_ParsesTheWire(t *testing.T) {
	f, c := newClient(t)
	action := uuid.New()

	f.route("GET /api/v1/tx/pending", f.incoming([]any{map[string]any{
		"id": action.String(), "purpose": "allocate", "description": "Allocate 0.01 CBTC",
		"hash": b64([]byte{1, 2, 3}), "createdAt": "2026-11-01T12:00:00Z", "expiresAt": "2026-11-01T12:10:00Z",
	}}))
	f.route("GET /api/v1/wallet/balances", f.incoming([]any{
		map[string]any{"symbol": "CBTC", "available": "0.5", "locked": "0.25"},
	}))
	f.route("GET /api/v1/wallet/incoming", f.incoming([]any{
		map[string]any{"contractId": "offer-1", "sender": "faucet", "symbol": "USDCx", "amount": "1000"},
	}))
	f.route("GET /api/v1/wallet/network", f.incoming(map[string]any{
		"name": "devnet", "faucet": []any{map[string]any{"symbol": "CBTC", "amount": "0.01"}},
	}))
	require.NoError(t, c.SignIn(t.Context()))

	actions, err := c.PendingActions(t.Context())
	require.NoError(t, err)
	require.Equal(t, []desk.Action{{ID: action, Purpose: "allocate", Description: "Allocate 0.01 CBTC", Hash: []byte{1, 2, 3}}}, actions)

	balances, err := c.Balances(t.Context())
	require.NoError(t, err)
	require.Len(t, balances, 1)
	require.Zero(t, balances["CBTC"].Cmp(big.NewRat(1, 2)), "only the available amount counts")

	transfers, err := c.IncomingTransfers(t.Context())
	require.NoError(t, err)
	require.Len(t, transfers, 1)
	require.Equal(t, "offer-1", transfers[0].ContractID)
	require.Zero(t, transfers[0].Amount.Cmp(big.NewRat(1000, 1)))

	network, err := c.Network(t.Context())
	require.NoError(t, err)
	require.Equal(t, "devnet", network.Name)
	require.Len(t, network.Faucet, 1)
	require.Zero(t, network.Faucet[0].Amount.Cmp(big.NewRat(1, 100)))
}

func TestExecute_SendsTheSignature(t *testing.T) {
	f, c := newClient(t)
	action := uuid.New()

	f.route("POST /api/v1/tx/"+action.String()+"/execute", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		require.Equal(t, map[string]string{"signature": b64([]byte{9, 9})}, body)
		w.WriteHeader(http.StatusNoContent)
	})
	require.NoError(t, c.SignIn(t.Context()))

	require.NoError(t, c.Execute(t.Context(), action, []byte{9, 9}))
}

func (f *fake) socket() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !f.session(r) {
			reply(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})

			return
		}

		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}

		_ = conn.Write(r.Context(), websocket.MessageText, []byte(`{"type":"sign.requested","data":{}}`))
		_ = conn.Close(websocket.StatusNormalClosure, "")
	}
}

func TestDial_SendsTheSessionCookie(t *testing.T) {
	f, c := newClient(t)
	f.route("GET /api/v1/ws", f.socket())
	require.NoError(t, c.SignIn(t.Context()))

	conn, err := c.Dial(t.Context())
	require.NoError(t, err)

	t.Cleanup(func() { _ = conn.CloseNow() })

	_, frame, err := conn.Read(t.Context())
	require.NoError(t, err)
	require.JSONEq(t, `{"type":"sign.requested","data":{}}`, string(frame))
}

func TestDial_SignsInAgainOn401(t *testing.T) {
	f, c := newClient(t)
	f.route("GET /api/v1/ws", f.socket())

	conn, err := c.Dial(t.Context())
	require.NoError(t, err)

	t.Cleanup(func() { _ = conn.CloseNow() })

	require.Equal(t, int64(1), f.logins.Load())
}

func TestDial_GivesUpOnAStalledUpgrade(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	f, srv := newFake(t, publicKey)
	f.route("GET /api/v1/ws", func(_ http.ResponseWriter, r *http.Request) { <-r.Context().Done() })

	c, err := desk.New(srv.URL, &http.Client{Transport: srv.Client().Transport, Timeout: 200 * time.Millisecond}, quietLog)
	require.NoError(t, err)
	c.SetSigner(party, func(message []byte) []byte { return ed25519.Sign(privateKey, message) })

	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()

	start := time.Now()
	_, err = c.Dial(ctx)
	require.Error(t, err)
	require.Less(t, time.Since(start), 2*time.Second, "a stalled upgrade ends with the client's request timeout")
}

func TestDial_TheConnectionOutlivesTheDialDeadline(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	f, srv := newFake(t, publicKey)
	f.route("GET /api/v1/ws", func(w http.ResponseWriter, r *http.Request) {
		if !f.session(r) {
			reply(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})

			return
		}

		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}

		time.Sleep(300 * time.Millisecond)
		_ = conn.Write(r.Context(), websocket.MessageText, []byte(`{"type":"wallet.updated","data":{}}`))
		_ = conn.Close(websocket.StatusNormalClosure, "")
	})

	c, err := desk.New(srv.URL, &http.Client{Transport: srv.Client().Transport, Timeout: 100 * time.Millisecond}, quietLog)
	require.NoError(t, err)
	c.SetSigner(party, func(message []byte) []byte { return ed25519.Sign(privateKey, message) })

	conn, err := c.Dial(t.Context())
	require.NoError(t, err)

	t.Cleanup(func() { _ = conn.CloseNow() })

	_, frame, err := conn.Read(t.Context())
	require.NoError(t, err, "the request timeout bounds the handshake, not the connection")
	require.JSONEq(t, `{"type":"wallet.updated","data":{}}`, string(frame))
}

func TestTransient(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	srv.Close()

	c, err := desk.New(srv.URL, srv.Client(), quietLog)
	require.NoError(t, err)

	_, err = c.IncomingRFQs(t.Context())
	require.True(t, desk.Transient(err), "a desk that does not answer is worth retrying: %v", err)

	require.True(t, desk.Transient(desk.ErrUnavailable))
	require.False(t, desk.Transient(desk.ErrConflict))
	require.False(t, desk.Transient(errors.New("bad input")))
}
