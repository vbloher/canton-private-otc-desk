package app_test

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/vbloher/canton-private-otc-desk/gen/grpc/identity_v1"
	"github.com/vbloher/canton-private-otc-desk/pkg/events"
	"github.com/vbloher/canton-private-otc-desk/pkg/httpserver"
	"github.com/vbloher/canton-private-otc-desk/pkg/otel"
	"github.com/vbloher/canton-private-otc-desk/pkg/principal"
	"github.com/vbloher/canton-private-otc-desk/pkg/redis"
	"github.com/vbloher/canton-private-otc-desk/services/gateway/config"
	"github.com/vbloher/canton-private-otc-desk/services/gateway/internal/adapter/identity"
	"github.com/vbloher/canton-private-otc-desk/services/gateway/internal/app"
	controller "github.com/vbloher/canton-private-otc-desk/services/gateway/internal/controller/http"
)

const wait = 10 * time.Second

// ranOnce: app.Run registers global Prometheus collectors, which panics on a second registration,
// so it can run once per process. `go test -count=N` repeats tests inside one process.
var ranOnce atomic.Bool //nolint:gochecknoglobals

// identityServer knows the sessions "alice-session" and "bob-session".
type identityServer struct {
	pb.UnimplementedIdentityV1Server

	users map[string]*pb.User
}

func (s *identityServer) Authenticate(_ context.Context, in *pb.AuthenticateInput) (*pb.User, error) {
	user, ok := s.users[in.GetSessionToken()]
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "unknown session")
	}

	return user, nil
}

func user(hint string) *pb.User {
	return &pb.User{Id: uuid.NewString(), PartyId: hint + "::1220ab", Hint: hint}
}

func listen(t *testing.T) net.Listener {
	t.Helper()

	var lc net.ListenConfig

	l, err := lc.Listen(t.Context(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)

	return l
}

func freePort(t *testing.T) string {
	t.Helper()

	l := listen(t)
	port := l.Addr().(*net.TCPAddr).Port //nolint:forcetypeassert

	require.NoError(t, l.Close())

	return strconv.Itoa(port)
}

// watch opens the socket and reports its frames (dropped when nobody reads) and how it ended.
func watch(t *testing.T, url, session string) (frames <-chan []byte, ended <-chan error) {
	t.Helper()

	conn, resp, err := websocket.Dial(t.Context(), url, &websocket.DialOptions{
		HTTPHeader: http.Header{"Cookie": []string{"otc_session=" + session}},
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusSwitchingProtocols, resp.StatusCode)

	t.Cleanup(func() { _ = conn.CloseNow() })

	f := make(chan []byte, 32)
	e := make(chan error, 1)

	go func() {
		for {
			_, payload, err := conn.Read(context.Background())
			if err != nil {
				e <- err

				return
			}

			select {
			case f <- payload:
			default:
			}
		}
	}()

	return f, e
}

// Test_Run_ServesProxyAndEvents starts the whole gateway on real sockets against a local Redis:
// requests are authenticated through a fake identity gRPC server and proxied to fake services, Redis events
// reach the WebSockets of the addressed party only, and cancelling the context shuts everything down.
func Test_Run_ServesProxyAndEvents(t *testing.T) {
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}

	probe, err := net.DialTimeout("tcp", redisAddr, time.Second)
	if err != nil {
		t.Skipf("needs Redis (make up), not reachable at %s: %v", redisAddr, err)
	}

	require.NoError(t, probe.Close())

	if !ranOnce.CompareAndSwap(false, true) {
		t.Skip("app.Run can run once per process (global Prometheus collectors)")
	}

	otel.SilentModeInit()

	alice, bob := user("alice-"+uuid.NewString()[:8]), user("bob-"+uuid.NewString()[:8])

	idp := listen(t)
	grpcServer := grpc.NewServer()
	pb.RegisterIdentityV1Server(grpcServer, &identityServer{users: map[string]*pb.User{
		"alice-session": alice,
		"bob-session":   bob,
	}})

	go func() { _ = grpcServer.Serve(idp) }()

	t.Cleanup(grpcServer.Stop)

	// Every service answers with the caller it was told about.
	service := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{
			"path":  r.URL.Path,
			"party": r.Header.Get(principal.HeaderPartyID),
		})
	}))
	t.Cleanup(service.Close)

	port := freePort(t)

	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)

	done := make(chan error, 1)

	go func() {
		done <- app.Run(ctx, config.Config{
			HTTP:      httpserver.Config{Port: port},
			Redis:     redis.Config{Addr: redisAddr},
			Identity:  identity.Config{Addr: idp.Addr().String()},
			Upstreams: controller.Config{IdentityURL: service.URL, TradingURL: service.URL, LedgerURL: service.URL},
		})
	}()

	base := "127.0.0.1:" + port

	require.Eventually(t, func() bool {
		resp, err := http.Get("http://" + base + "/live") //nolint:noctx,bodyclose // polling until the port is open
		if err != nil {
			return false
		}

		_ = resp.Body.Close()

		return resp.StatusCode == http.StatusOK
	}, wait, 50*time.Millisecond, "the gateway did not start")

	// Authentication and proxying.
	get := func(session string) (int, map[string]string) {
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+base+"/api/v1/trades", nil)
		require.NoError(t, err)

		req.AddCookie(&http.Cookie{Name: "otc_session", Value: session})

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)

		defer func() { _ = resp.Body.Close() }()

		var body map[string]string

		_ = json.NewDecoder(resp.Body).Decode(&body)

		return resp.StatusCode, body
	}

	code, body := get("alice-session")
	require.Equal(t, http.StatusOK, code)
	require.Equal(t, map[string]string{"path": "/api/v1/trades", "party": alice.GetPartyId()}, body)

	code, _ = get("forged-session")
	require.Equal(t, http.StatusUnauthorized, code)

	// Events: Redis -> subscriber -> hub -> the party's socket.
	aliceFrames, aliceEnded := watch(t, "ws://"+base+"/api/v1/ws", "alice-session")
	bobFrames, bobEnded := watch(t, "ws://"+base+"/api/v1/ws", "bob-session")

	redisClient, err := redis.New(ctx, redis.Config{Addr: redisAddr})
	require.NoError(t, err)

	t.Cleanup(redisClient.Close)

	publisher := events.NewPublisher(redisClient)

	forAlice := events.Event{Type: events.TradeUpdated, Data: map[string]string{"tradeId": "t1", "rfqId": "r1", "status": "settled"}}
	wantAlice, err := json.Marshal(forAlice)
	require.NoError(t, err)

	// The sockets are registered a moment after the upgrade and the subscription a moment after start-up
	// (delivery is at-most-once), so keep publishing until the first event arrives.
	var gotAlice []byte

	require.Eventually(t, func() bool {
		require.NoError(t, publisher.Publish(ctx, forAlice, alice.GetPartyId()))

		select {
		case gotAlice = <-aliceFrames:
			return true
		default:
			return false
		}
	}, wait, 50*time.Millisecond, "alice never got her event")
	require.JSONEq(t, string(wantAlice), string(gotAlice))
	require.Equal(t, string(wantAlice), string(gotAlice), "forwarded verbatim")

	// Bob has been connected all along and was not addressed: his first frame is the one meant for him.
	forBob := events.Event{Type: events.WalletUpdated, Data: struct{}{}}
	wantBob, err := json.Marshal(forBob)
	require.NoError(t, err)

	var gotBob []byte

	require.Eventually(t, func() bool {
		require.NoError(t, publisher.Publish(ctx, forBob, bob.GetPartyId()))

		select {
		case gotBob = <-bobFrames:
			return true
		default:
			return false
		}
	}, wait, 50*time.Millisecond, "bob never got his event")
	require.Equal(t, string(wantBob), string(gotBob), "bob must not see alice's events")

	// Shutdown: the context ends, sockets get a going-away close, Run returns cleanly.
	cancel()

	for name, ended := range map[string]<-chan error{"alice": aliceEnded, "bob": bobEnded} {
		select {
		case err := <-ended:
			require.Equal(t, websocket.StatusGoingAway, websocket.CloseStatus(err), name)
		case <-time.After(wait):
			require.FailNow(t, name+"'s socket was not closed on shutdown")
		}
	}

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(wait):
		require.FailNow(t, "app.Run did not return after the context ended")
	}
}
