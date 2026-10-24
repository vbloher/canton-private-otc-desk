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

// socketGateway is the whole router (all middlewares) on a real listener. No service is needed for the socket.
type socketGateway struct {
	url     string // ws://host/api/v1/ws
	host    string // host:port, what a same-origin page would send as its Origin
	deliver func(partyID string, payload []byte)
}

func serveSocketGateway(t *testing.T, idp *mocks.Identity, options ...func(*controller.Config)) socketGateway {
	t.Helper()

	cfg := controller.Config{IdentityURL: "http://127.0.0.1:1", TradingURL: "http://127.0.0.1:1", LedgerURL: "http://127.0.0.1:1"}
	for _, option := range options {
		option(&cfg)
	}

	h, hub := newRouter(t, idp, cfg)

	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	return socketGateway{
		url:     "ws" + strings.TrimPrefix(srv.URL, "http") + "/api/v1/ws",
		host:    strings.TrimPrefix(srv.URL, "http://"),
		deliver: hub.Deliver,
	}
}

// dial opens the socket; opts carries the headers and subprotocols of the client.
func (g socketGateway) dial(t *testing.T, opts *websocket.DialOptions) (*websocket.Conn, *http.Response, error) {
	t.Helper()

	conn, resp, err := websocket.Dial(t.Context(), g.url, opts)
	if conn != nil {
		t.Cleanup(func() { _ = conn.CloseNow() })
	}

	return conn, resp, err
}

func (g socketGateway) mustDial(t *testing.T, opts *websocket.DialOptions) (*websocket.Conn, *http.Response) {
	t.Helper()

	conn, resp, err := g.dial(t, opts)
	require.NoError(t, err)
	require.Equal(t, http.StatusSwitchingProtocols, resp.StatusCode)

	return conn, resp
}

// firstEvent publishes an event for each party in turn until a frame arrives on conn and returns that frame.
// The upgrade completes a moment before the socket is registered (events are at-most-once), hence the retries.
// Every party gets its own payload, so the frame also tells whose socket this is.
func (g socketGateway) firstEvent(t *testing.T, conn *websocket.Conn, payloads map[string][]byte) []byte {
	t.Helper()

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

	var got []byte

	require.Eventually(t, func() bool {
		for partyID, payload := range payloads {
			g.deliver(partyID, payload)
		}

		select {
		case got = <-frames:
			return true
		default:
			return false
		}
	}, 5*time.Second, 10*time.Millisecond, "no event arrived")

	return got
}

var (
	forAlice = []byte(`{"type":"trade.updated","data":{"tradeId":"t1","rfqId":"r1","status":"settled"}}`)
	forBob   = []byte(`{"type":"wallet.updated","data":{}}`)
	forMaker = []byte(`{"type":"rfq.created","data":{"rfqId":"r2"}}`)
)

func allParties() map[string][]byte {
	return map[string][]byte{alice.PartyID: forAlice, bob.PartyID: forBob, maker.PartyID: forMaker}
}

func Test_WS_DeliversEventsOfTheCaller(t *testing.T) {
	tests := map[string]*websocket.DialOptions{
		"browser session": {HTTPHeader: http.Header{"Cookie": []string{domain.SessionCookie + "=alice-session"}}},
		"bot api key":     {HTTPHeader: http.Header{domain.APIKeyHeader: []string{"alice-key"}}},
		"bearer header":   {HTTPHeader: http.Header{"Authorization": []string{"Bearer alice-session"}}},
	}

	for name, opts := range tests {
		t.Run(name, func(t *testing.T) {
			g := serveSocketGateway(t, identityKnowsAlice(t))

			conn, _ := g.mustDial(t, opts)

			require.Equal(t, forAlice, g.firstEvent(t, conn, allParties()), "the frame is the Redis payload, unchanged, and only hers")
		})
	}
}

func Test_WS_RequiresCaller(t *testing.T) {
	idp := mocks.NewIdentity(t)
	idp.EXPECT().Authenticate(Any, "expired").Return(principal.Principal{}, domain.ErrUnauthorized)
	idp.EXPECT().Authenticate(Any, "forged").Return(principal.Principal{}, domain.ErrUnauthorized)
	idp.EXPECT().AuthenticateAPIKey(Any, "wrong").Return(principal.Principal{}, domain.ErrUnauthorized)

	g := serveSocketGateway(t, idp)

	tests := map[string]*websocket.DialOptions{
		"no credentials":               {},
		"expired session":              {HTTPHeader: http.Header{"Cookie": []string{domain.SessionCookie + "=expired"}}},
		"unknown api key":              {HTTPHeader: http.Header{domain.APIKeyHeader: []string{"wrong"}}},
		"forged bearer header":         {HTTPHeader: http.Header{"Authorization": []string{"Bearer forged"}}},
		"forged bearer subprotocol":    {Subprotocols: []string{"otc-desk.v1", "otc-desk.bearer.forged"}},
		"malformed bearer subprotocol": {Subprotocols: []string{"otc-desk.v1", "otc-desk.bearer."}},
		"junk: padding in the middle":  {Subprotocols: []string{"otc-desk.v1", "otc-desk.bearer.ab=cd"}},
		"junk: only padding":           {Subprotocols: []string{"otc-desk.v1", "otc-desk.bearer.=="}},
		"junk: a space":                {Subprotocols: []string{"otc-desk.v1", "otc-desk.bearer.ab cd"}},
		"junk: quotes":                 {Subprotocols: []string{"otc-desk.v1", `otc-desk.bearer."forged"`}},
		"junk: not ASCII":              {Subprotocols: []string{"otc-desk.v1", "otc-desk.bearer.tökén"}},
		"only the protocol":            {Subprotocols: []string{"otc-desk.v1"}},
		"somebody else's protocol":     {Subprotocols: []string{"chat", "bearer.forged"}},
	}

	for name, opts := range tests {
		t.Run(name, func(t *testing.T) {
			_, resp, err := g.dial(t, opts)

			require.Error(t, err)
			require.NotNil(t, resp)
			require.Equal(t, http.StatusUnauthorized, resp.StatusCode, "no upgrade without a caller")
		})
	}
}

// Browsers cannot set headers on a WebSocket, so the desk's page offers its session as a subprotocol.
func Test_WS_AuthenticatesWithTheSubprotocolBearer(t *testing.T) {
	idp := mocks.NewIdentity(t)
	idp.EXPECT().Authenticate(Any, "alice-session").Return(alice, nil)

	g := serveSocketGateway(t, idp)

	conn, _ := g.mustDial(t, &websocket.DialOptions{Subprotocols: []string{"otc-desk.v1", "otc-desk.bearer.alice-session"}})

	require.Equal(t, forAlice, g.firstEvent(t, conn, allParties()))
}

// A subprotocol token that is not even shaped like a token is no credential, exactly like a malformed
// Authorization header: it is not looked up, and whatever else the client presented still counts.
func Test_WS_JunkSubprotocolTokenCountsAsAbsent(t *testing.T) {
	junk := []string{"otc-desk.bearer.ab=cd", "otc-desk.bearer.==", "otc-desk.bearer.ab cd", `otc-desk.bearer."x"`}

	for _, entry := range junk {
		t.Run(entry+"/with a cookie", func(t *testing.T) {
			// A strict mock: only the cookie may be looked up, never the junk.
			idp := mocks.NewIdentity(t)
			idp.EXPECT().Authenticate(Any, "cookie-token").Return(bob, nil)

			g := serveSocketGateway(t, idp)

			conn, _ := g.mustDial(t, &websocket.DialOptions{
				Subprotocols: []string{"otc-desk.v1", entry},
				HTTPHeader:   http.Header{"Cookie": []string{domain.SessionCookie + "=cookie-token"}},
			})

			require.Equal(t, forBob, g.firstEvent(t, conn, allParties()))
		})
	}

	t.Run("a junk Authorization header and a good subprotocol", func(t *testing.T) {
		idp := mocks.NewIdentity(t)
		idp.EXPECT().Authenticate(Any, "sub-token").Return(alice, nil)

		g := serveSocketGateway(t, idp)

		conn, _ := g.mustDial(t, &websocket.DialOptions{
			Subprotocols: []string{"otc-desk.v1", "otc-desk.bearer.sub-token"},
			HTTPHeader:   http.Header{"Authorization": []string{"Bearer not a token"}},
		})

		require.Equal(t, forAlice, g.firstEvent(t, conn, allParties()))
	})
}

// X-API-Key > bearer (the header, then the subprotocol) > cookie. A strict mock fails the test if a credential
// that loses is looked up, and the event that arrives names the party the socket was registered for.
func Test_WS_CredentialPrecedence(t *testing.T) {
	tests := map[string]struct {
		opts    websocket.DialOptions
		arrange func(*mocks.Identity)
		want    []byte
	}{
		"api key beats everything": {
			opts: websocket.DialOptions{
				Subprotocols: []string{"otc-desk.v1", "otc-desk.bearer.sub-token"},
				HTTPHeader: http.Header{
					domain.APIKeyHeader: []string{"the-key"},
					"Authorization":     []string{"Bearer header-token"},
					"Cookie":            []string{domain.SessionCookie + "=cookie-token"},
				},
			},
			arrange: func(m *mocks.Identity) { m.EXPECT().AuthenticateAPIKey(Any, "the-key").Return(maker, nil) },
			want:    forMaker,
		},
		"bearer header beats the subprotocol": {
			opts: websocket.DialOptions{
				Subprotocols: []string{"otc-desk.v1", "otc-desk.bearer.sub-token"},
				HTTPHeader: http.Header{
					"Authorization": []string{"Bearer header-token"},
					"Cookie":        []string{domain.SessionCookie + "=cookie-token"},
				},
			},
			arrange: func(m *mocks.Identity) { m.EXPECT().Authenticate(Any, "header-token").Return(alice, nil) },
			want:    forAlice,
		},
		"subprotocol beats the cookie": {
			opts: websocket.DialOptions{
				Subprotocols: []string{"otc-desk.v1", "otc-desk.bearer.sub-token"},
				HTTPHeader:   http.Header{"Cookie": []string{domain.SessionCookie + "=cookie-token"}},
			},
			arrange: func(m *mocks.Identity) { m.EXPECT().Authenticate(Any, "sub-token").Return(alice, nil) },
			want:    forAlice,
		},
		"cookie when there is nothing else": {
			opts: websocket.DialOptions{
				Subprotocols: []string{"otc-desk.v1"},
				HTTPHeader:   http.Header{"Cookie": []string{domain.SessionCookie + "=cookie-token"}},
			},
			arrange: func(m *mocks.Identity) { m.EXPECT().Authenticate(Any, "cookie-token").Return(bob, nil) },
			want:    forBob,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			idp := mocks.NewIdentity(t)
			tt.arrange(idp)

			g := serveSocketGateway(t, idp)

			conn, _ := g.mustDial(t, &tt.opts)

			require.Equal(t, tt.want, g.firstEvent(t, conn, allParties()))
		})
	}
}

// A browser fails the handshake when it offered subprotocols and the server picked none, so the desk's protocol is
// always echoed. The entry that carries the session must never come back: it would put the secret in a response.
func Test_WS_NegotiatesTheProtocol(t *testing.T) {
	tests := map[string]struct {
		subprotocols []string
		wantEcho     []string
	}{
		"protocol and bearer":         {[]string{"otc-desk.v1", "otc-desk.bearer.alice-session"}, []string{"otc-desk.v1"}},
		"bearer listed first":         {[]string{"otc-desk.bearer.alice-session", "otc-desk.v1"}, []string{"otc-desk.v1"}},
		"protocol only":               {[]string{"otc-desk.v1"}, []string{"otc-desk.v1"}},
		"with a stranger":             {[]string{"chat", "otc-desk.v1", "otc-desk.bearer.alice-session"}, []string{"otc-desk.v1"}},
		"bearer without the protocol": {[]string{"otc-desk.bearer.alice-session"}, nil},
		"nothing offered":             {nil, nil},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			g := serveSocketGateway(t, identityKnowsAlice(t))

			conn, resp := g.mustDial(t, &websocket.DialOptions{
				Subprotocols: tt.subprotocols,
				HTTPHeader:   http.Header{"Cookie": []string{domain.SessionCookie + "=alice-session"}},
			})

			require.Equal(t, tt.wantEcho, resp.Header.Values("Sec-WebSocket-Protocol"))

			if len(tt.wantEcho) > 0 {
				require.Equal(t, "otc-desk.v1", conn.Subprotocol())
			} else {
				require.Empty(t, conn.Subprotocol())
			}
		})
	}
}

func Test_WS_Origins(t *testing.T) {
	bearer := []string{"otc-desk.v1", "otc-desk.bearer.alice-session"}

	withAndWithoutCORS := map[string][]func(*controller.Config){
		"cors off": nil,
		"cors on":  {withCORS(pagesOrigin)},
	}

	t.Run("a configured origin may connect", func(t *testing.T) {
		g := serveSocketGateway(t, identityKnowsAlice(t), withCORS(pagesOrigin, "http://localhost:5173"))

		for _, origin := range []string{pagesOrigin, "http://localhost:5173"} {
			g.mustDial(t, &websocket.DialOptions{Subprotocols: bearer, HTTPHeader: http.Header{"Origin": []string{origin}}})
		}
	})

	t.Run("an ipv6 origin may connect", func(t *testing.T) {
		g := serveSocketGateway(t, identityKnowsAlice(t), withCORS("http://[::1]:5173"))

		g.mustDial(t, &websocket.DialOptions{Subprotocols: bearer, HTTPHeader: http.Header{"Origin": []string{"http://[::1]:5173"}}})
	})

	for name, options := range withAndWithoutCORS {
		t.Run("the gateway's own origin may connect, "+name, func(t *testing.T) {
			g := serveSocketGateway(t, identityKnowsAlice(t), options...)

			g.mustDial(t, &websocket.DialOptions{
				HTTPHeader: http.Header{"Origin": []string{"http://" + g.host}, "Cookie": []string{domain.SessionCookie + "=alice-session"}},
			})
		})
	}

	t.Run("no origin at all (a bot) may connect", func(t *testing.T) {
		g := serveSocketGateway(t, identityKnowsAlice(t), withCORS(pagesOrigin))

		g.mustDial(t, &websocket.DialOptions{HTTPHeader: http.Header{domain.APIKeyHeader: []string{"alice-key"}}})
	})

	refused := map[string]string{
		"an unrelated site": "https://evil.example",
		"a suffix trick":    "https://vbloher.github.io.evil.example",
		"another scheme":    "http://vbloher.github.io",
		"another port":      "https://vbloher.github.io:8443",
		"a subdomain":       "https://app.vbloher.github.io",
		"opaque":            "null",
	}

	for name, origin := range refused {
		for variant, options := range withAndWithoutCORS {
			t.Run("refused: "+name+", "+variant, func(t *testing.T) {
				g := serveSocketGateway(t, identityKnowsAlice(t), options...)

				// Even with a valid session: the page of another site must not be able to ride it.
				_, resp, err := g.dial(t, &websocket.DialOptions{
					Subprotocols: bearer,
					HTTPHeader:   http.Header{"Origin": []string{origin}},
				})

				require.Error(t, err)
				require.NotNil(t, resp)
				require.Equal(t, http.StatusForbidden, resp.StatusCode)
			})
		}
	}
}
