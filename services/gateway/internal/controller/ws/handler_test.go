package ws_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	neturl "net/url"
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
func serve(t *testing.T, hub *ws.Hub, who *principal.Principal, originPatterns ...string) string {
	t.Helper()

	handler := ws.NewHandler(hub, originPatterns...)

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

// dialWith is dial for a caller that wants the handshake response as well.
func dialWith(t *testing.T, url string, opts *websocket.DialOptions) (*websocket.Conn, *http.Response) {
	t.Helper()

	conn, resp, err := websocket.Dial(t.Context(), url, opts)
	require.NoError(t, err)
	require.Equal(t, http.StatusSwitchingProtocols, resp.StatusCode)

	t.Cleanup(func() { _ = conn.CloseNow() })

	return conn, resp
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

func Test_Handler_NegotiatesTheProtocol(t *testing.T) {
	tests := map[string]struct {
		offered []string
		want    []string
	}{
		"protocol and bearer":         {[]string{"otc-desk.v1", "otc-desk.bearer.secret-session"}, []string{"otc-desk.v1"}},
		"bearer first":                {[]string{"otc-desk.bearer.secret-session", "otc-desk.v1"}, []string{"otc-desk.v1"}},
		"protocol only":               {[]string{"otc-desk.v1"}, []string{"otc-desk.v1"}},
		"a stranger and the protocol": {[]string{"chat", "otc-desk.v1"}, []string{"otc-desk.v1"}},
		"bearer without the protocol": {[]string{"otc-desk.bearer.secret-session"}, nil},
		"strangers only":              {[]string{"chat", "superchat"}, nil},
		"nothing":                     {nil, nil},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			hub := newHub(t, quiet)
			conn, resp := dialWith(t, serve(t, hub, &alice), &websocket.DialOptions{Subprotocols: tt.offered})

			require.Equal(t, tt.want, resp.Header.Values("Sec-WebSocket-Protocol"), "only the desk's protocol is ever echoed")
			require.Equal(t, strings.Join(tt.want, ""), conn.Subprotocol())
		})
	}
}

func Test_Handler_AcceptsTheGivenOriginsAndTheOwnOne(t *testing.T) {
	hub := newHub(t, quiet)
	url := serve(t, hub, &alice, "https://pages.example", "http://localhost:5173")

	for _, origin := range []string{"https://pages.example", "http://localhost:5173", "https://PAGES.example"} {
		dialWith(t, url, &websocket.DialOptions{HTTPHeader: http.Header{"Origin": []string{origin}}})
	}

	own, err := neturl.Parse(url)
	require.NoError(t, err)

	dialWith(t, url, &websocket.DialOptions{HTTPHeader: http.Header{"Origin": []string{"http://" + own.Host}}})

	for _, origin := range []string{"https://evil.example", "http://pages.example", "https://pages.example:8443", "https://x.pages.example"} {
		_, resp, err := websocket.Dial(t.Context(), url, &websocket.DialOptions{HTTPHeader: http.Header{"Origin": []string{origin}}})

		require.Error(t, err, origin)
		require.NotNil(t, resp, origin)
		require.Equal(t, http.StatusForbidden, resp.StatusCode, origin)
	}
}

func Test_BearerToken(t *testing.T) {
	tests := map[string]struct {
		header []string // the Sec-WebSocket-Protocol header lines
		want   string
	}{
		"after the protocol":             {[]string{"otc-desk.v1, otc-desk.bearer.abc-DEF_123"}, "abc-DEF_123"},
		"before the protocol":            {[]string{"otc-desk.bearer.abc, otc-desk.v1"}, "abc"},
		"in a header line of its own":    {[]string{"otc-desk.v1", "otc-desk.bearer.abc"}, "abc"},
		"padded with spaces":             {[]string{"  otc-desk.v1 ,   otc-desk.bearer.abc  "}, "abc"},
		"the first of two":               {[]string{"otc-desk.bearer.first, otc-desk.bearer.second"}, "first"},
		"a token with dots":              {[]string{"otc-desk.bearer.a.b.c"}, "a.b.c"},
		"among strangers":                {[]string{"chat, otc-desk.bearer.abc, superchat"}, "abc"},
		"no bearer entry":                {[]string{"otc-desk.v1"}, ""},
		"no header":                      {nil, ""},
		"empty token":                    {[]string{"otc-desk.v1, otc-desk.bearer."}, ""},
		"the prefix inside another name": {[]string{"x-otc-desk.bearer.abc"}, ""},
		"look-alikes":                    {[]string{"otc-desk.bearers.abc, otc-desk.bearer, bearer.abc"}, ""},

		// The same syntax as the Authorization header (RFC 7235 token68): anything else is as good as absent.
		"every symbol and padding":         {[]string{"otc-desk.bearer.a-b.c_d~e+f/g=="}, "a-b.c_d~e+f/g=="},
		"padding in the middle":            {[]string{"otc-desk.bearer.ab=cd"}, ""},
		"only padding":                     {[]string{"otc-desk.bearer.=="}, ""},
		"a space inside":                   {[]string{"otc-desk.bearer.ab cd"}, ""},
		"quotes":                           {[]string{`otc-desk.bearer."abc"`}, ""},
		"a colon":                          {[]string{"otc-desk.bearer.ab:cd"}, ""},
		"not ASCII":                        {[]string{"otc-desk.bearer.tökén"}, ""},
		"junk is skipped, the next counts": {[]string{"otc-desk.bearer.a b, otc-desk.bearer.==, otc-desk.bearer.good"}, "good"},
		"junk and nothing else":            {[]string{"otc-desk.v1, otc-desk.bearer.a b"}, ""},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/ws", nil)
			for _, line := range tt.header {
				r.Header.Add("Sec-WebSocket-Protocol", line)
			}

			require.Equal(t, tt.want, ws.BearerToken(r))
		})
	}
}
