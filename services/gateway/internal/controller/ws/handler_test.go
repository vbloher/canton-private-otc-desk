package ws_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/pkg/principal"
	"github.com/vbloher/canton-private-otc-desk/services/gateway/internal/controller/ws"
)

var alice = principal.Principal{UserID: uuid.New(), PartyID: "alice::1220", Hint: "alice"}

// serve runs the handler the way the router does: behind an authentication step that puts the caller
// into the request context (nobody when who is nil). It returns the ws:// URL.
func serve(t *testing.T, hub *ws.Hub, who *principal.Principal) string {
	t.Helper()

	handler := ws.NewHandler(hub)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if who != nil {
			r = r.WithContext(principal.WithContext(r.Context(), *who))
		}

		handler.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)

	return "ws" + strings.TrimPrefix(srv.URL, "http")
}

func dial(t *testing.T, url string, opts *websocket.DialOptions) *websocket.Conn {
	t.Helper()

	conn, resp, err := websocket.Dial(t.Context(), url, opts)
	require.NoError(t, err)
	require.Equal(t, http.StatusSwitchingProtocols, resp.StatusCode)

	t.Cleanup(func() { _ = conn.CloseNow() })

	return conn
}

func registered(hub *ws.Hub, n int) func() bool {
	return func() bool { return hub.Len() == n }
}

func Test_Handler_ServesEventsOfTheCallersParty(t *testing.T) {
	hub := newHub(t, quiet)
	conn := dial(t, serve(t, hub, &alice), nil)

	// The upgrade completes before the socket is registered; give the handler a moment.
	require.Eventually(t, registered(hub, 1), wait, tick)

	mine := []byte(`{"type":"quote.updated","data":{"quoteId":"q1","rfqId":"r1","status":"open"}}`)

	hub.Deliver("bob::1220", []byte(`{"type":"wallet.updated","data":{}}`))
	hub.Deliver(alice.PartyID, mine)

	typ, got, err := conn.Read(t.Context())
	require.NoError(t, err)
	require.Equal(t, websocket.MessageText, typ)
	require.Equal(t, mine, got, "the first frame is the caller's own event, unchanged")
}

func Test_Handler_ForgetsSocketWhenClientLeaves(t *testing.T) {
	hub := newHub(t, quiet)
	conn := dial(t, serve(t, hub, &alice), nil)

	require.Eventually(t, registered(hub, 1), wait, tick)

	require.NoError(t, conn.Close(websocket.StatusNormalClosure, "bye"))

	require.Eventually(t, registered(hub, 0), wait, tick)
}

func Test_Handler_KeepsAnsweringPings(t *testing.T) {
	hub := newHub(t, 20*time.Millisecond)

	var pings atomic.Int64

	conn := dial(t, serve(t, hub, &alice), &websocket.DialOptions{
		OnPingReceived: func(context.Context, []byte) bool {
			pings.Add(1)

			return true
		},
	})

	// The client answers pings while it reads.
	go func() {
		for {
			_, _, err := conn.Read(context.Background())
			if err != nil {
				return
			}
		}
	}()

	// The hub sends the next ping only after the previous pong was read, so several pings in a short time
	// mean the server really consumes pongs; a socket that did not would be dropped after the pong timeout.
	require.Eventually(t, func() bool { return pings.Load() >= 3 }, wait, tick)
	require.Equal(t, 1, hub.Len())
}

func Test_Handler_RefusesWithoutCaller(t *testing.T) {
	hub := newHub(t, quiet)

	conn, resp, err := websocket.Dial(t.Context(), serve(t, hub, nil), nil)
	if conn != nil {
		_ = conn.CloseNow()
	}

	require.Error(t, err)
	require.NotNil(t, resp)
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	require.Equal(t, 0, hub.Len())
}

func Test_Handler_RefusesForeignOrigin(t *testing.T) {
	hub := newHub(t, quiet)

	conn, resp, err := websocket.Dial(t.Context(), serve(t, hub, &alice), &websocket.DialOptions{
		HTTPHeader: http.Header{"Origin": []string{"https://evil.example"}},
	})
	if conn != nil {
		_ = conn.CloseNow()
	}

	require.Error(t, err)
	require.NotNil(t, resp)
	require.Equal(t, http.StatusForbidden, resp.StatusCode, "a page from another site cannot ride the user's cookie")
	require.Equal(t, 0, hub.Len())
}

func Test_Handler_HubShutdownClosesClient(t *testing.T) {
	hub := ws.NewHub(quiet)
	conn := dial(t, serve(t, hub, &alice), nil)

	require.Eventually(t, registered(hub, 1), wait, tick)

	// A browser answers the close frame on its own; here the client has to be reading for that.
	closed := make(chan error, 1)

	go func() {
		_, _, err := conn.Read(context.Background())
		closed <- err
	}()

	hub.Close()

	select {
	case err := <-closed:
		require.Equal(t, websocket.StatusGoingAway, websocket.CloseStatus(err))
	case <-time.After(wait):
		require.FailNow(t, "the client was not told the server is going away")
	}

	require.Equal(t, 0, hub.Len())
}
