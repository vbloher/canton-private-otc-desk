package http_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/pkg/principal"
	controller "github.com/vbloher/canton-private-otc-desk/services/gateway/internal/controller/http"
	"github.com/vbloher/canton-private-otc-desk/services/gateway/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/gateway/internal/usecase/mocks"
)

func serveSocketGateway(t *testing.T, idp *mocks.Identity) (url string, hubDeliver func(partyID string, payload []byte)) {
	t.Helper()

	h, hub := newRouter(t, idp, controller.Config{
		IdentityURL: "http://127.0.0.1:1", TradingURL: "http://127.0.0.1:1", LedgerURL: "http://127.0.0.1:1",
	})

	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	return "ws" + strings.TrimPrefix(srv.URL, "http") + "/api/v1/ws", hub.Deliver
}

func Test_WS_DeliversEventsOfTheCaller(t *testing.T) {
	tests := map[string]http.Header{
		"browser session": {"Cookie": []string{domain.SessionCookie + "=alice-session"}},
		"bot api key":     {domain.APIKeyHeader: []string{"alice-key"}},
	}

	for name, header := range tests {
		t.Run(name, func(t *testing.T) {
			url, deliver := serveSocketGateway(t, identityKnowsAlice(t))

			conn, resp, err := websocket.Dial(t.Context(), url, &websocket.DialOptions{HTTPHeader: header})
			require.NoError(t, err)
			require.Equal(t, http.StatusSwitchingProtocols, resp.StatusCode)

			t.Cleanup(func() { _ = conn.CloseNow() })

			frames := make(chan []byte, 16)

			go func() {
				for {
					typ, payload, err := conn.Read(t.Context())
					if err != nil || typ != websocket.MessageText {
						return
					}

					frames <- payload
				}
			}()

			mine := []byte(`{"type":"trade.updated","data":{"tradeId":"t1","rfqId":"r1","status":"settled"}}`)
			theirs := []byte(`{"type":"wallet.updated","data":{}}`)

			var got []byte

			require.Eventually(t, func() bool {
				deliver("bob::1220cd", theirs)
				deliver(alice.PartyID, mine)

				select {
				case got = <-frames:
					return true
				default:
					return false
				}
			}, 5*time.Second, 10*time.Millisecond)

			require.Equal(t, mine, got, "the frame is the Redis payload, unchanged")
		})
	}
}

func Test_WS_RequiresCaller(t *testing.T) {
	idp := mocks.NewIdentity(t)
	idp.EXPECT().Authenticate(Any, "expired").Return(principal.Principal{}, domain.ErrUnauthorized)
	idp.EXPECT().AuthenticateAPIKey(Any, "wrong").Return(principal.Principal{}, domain.ErrUnauthorized)

	url, _ := serveSocketGateway(t, idp)

	tests := map[string]http.Header{
		"no credentials":  {},
		"expired session": {"Cookie": []string{domain.SessionCookie + "=expired"}},
		"unknown api key": {domain.APIKeyHeader: []string{"wrong"}},
	}

	for name, header := range tests {
		t.Run(name, func(t *testing.T) {
			conn, resp, err := websocket.Dial(t.Context(), url, &websocket.DialOptions{HTTPHeader: header})
			if conn != nil {
				_ = conn.CloseNow()
			}

			require.Error(t, err)
			require.NotNil(t, resp)
			require.Equal(t, http.StatusUnauthorized, resp.StatusCode, "no upgrade without a caller")
		})
	}
}
