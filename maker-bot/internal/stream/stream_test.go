package stream_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/maker-bot/internal/stream"
)

const (
	wait = 5 * time.Second
	tick = 5 * time.Millisecond
)

var quietLog = slog.New(slog.NewTextHandler(io.Discard, nil))

type recorder struct {
	mu        sync.Mutex
	connected int
	events    []stream.Event
}

func (r *recorder) Connected(context.Context) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.connected++
}

func (r *recorder) Event(_ context.Context, ev stream.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.events = append(r.events, ev)
}

func (r *recorder) seen() (int, []stream.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.connected, append([]stream.Event(nil), r.events...)
}

func serve(t *testing.T, handle func(ctx context.Context, conn *websocket.Conn)) (func(context.Context) (*websocket.Conn, error), *atomic.Int64) {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}

		defer func() { _ = conn.CloseNow() }()

		handle(r.Context(), conn)
	}))
	t.Cleanup(srv.Close)

	var dials atomic.Int64

	url := "ws" + strings.TrimPrefix(srv.URL, "http")

	return func(ctx context.Context) (*websocket.Conn, error) {
		dials.Add(1)

		conn, resp, err := websocket.Dial(ctx, url, nil)
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}

		return conn, err
	}, &dials
}

func run(t *testing.T, dial func(context.Context) (*websocket.Conn, error), h stream.Handler, opts stream.Options) (stop func() error) {
	t.Helper()

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)

	go func() { done <- stream.Run(ctx, dial, h, opts, quietLog) }()

	return func() error {
		cancel()

		select {
		case err := <-done:
			return err
		case <-time.After(wait):
			require.FailNow(t, "Run did not stop")

			return nil
		}
	}
}

func write(ctx context.Context, conn *websocket.Conn, frames ...string) {
	for _, frame := range frames {
		_ = conn.Write(ctx, websocket.MessageText, []byte(frame))
	}
}

func TestRun_DeliversEventsInOrder(t *testing.T) {
	dial, _ := serve(t, func(ctx context.Context, conn *websocket.Conn) {
		write(ctx, conn,
			`{"type":"rfq.created","data":{"rfqId":"7"}}`,
			`{"type":"sign.requested","data":{}}`,
		)

		_, _, _ = conn.Read(ctx)
	})

	h := &recorder{}
	stop := run(t, dial, h, stream.Options{})

	require.Eventually(t, func() bool { _, events := h.seen(); return len(events) == 2 }, wait, tick)
	require.ErrorIs(t, stop(), context.Canceled)

	connected, events := h.seen()
	require.Equal(t, 1, connected)
	require.Equal(t, "rfq.created", events[0].Type)
	require.JSONEq(t, `{"rfqId":"7"}`, string(events[0].Data))
	require.Equal(t, "sign.requested", events[1].Type)
}

func TestRun_CallsConnectedOnEveryConnect(t *testing.T) {
	dial, dials := serve(t, func(ctx context.Context, conn *websocket.Conn) {
		write(ctx, conn, `{"type":"wallet.updated","data":{}}`)
		_ = conn.Close(websocket.StatusNormalClosure, "")
	})

	h := &recorder{}
	stop := run(t, dial, h, stream.Options{Backoff: 10 * time.Millisecond})

	require.Eventually(t, func() bool { connected, _ := h.seen(); return connected >= 2 }, wait, tick)
	require.ErrorIs(t, stop(), context.Canceled)
	require.GreaterOrEqual(t, dials.Load(), int64(2))
}

func TestRun_ReconnectsWhenPingsGoUnanswered(t *testing.T) {
	dial, dials := serve(t, func(ctx context.Context, _ *websocket.Conn) {
		<-ctx.Done()
	})

	h := &recorder{}
	stop := run(t, dial, h, stream.Options{
		Backoff: 10 * time.Millisecond, PingInterval: 20 * time.Millisecond, PingTimeout: 50 * time.Millisecond,
	})

	require.Eventually(t, func() bool { return dials.Load() >= 2 }, wait, tick, "a socket that never answers a ping is dialled again")
	require.ErrorIs(t, stop(), context.Canceled)
}

func TestRun_KeepsReadingWhileConnectedRuns(t *testing.T) {
	var pinged atomic.Bool

	dial, _ := serve(t, func(ctx context.Context, conn *websocket.Conn) {
		go func() {
			for {
				if _, _, err := conn.Read(ctx); err != nil {
					return
				}
			}
		}()

		pingCtx, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()

		pinged.Store(conn.Ping(pingCtx) == nil)
		<-ctx.Done()
	})

	h := &slowConnect{release: make(chan struct{})}
	stop := run(t, dial, h, stream.Options{})

	require.Eventually(t, pinged.Load, wait, tick, "the desk's pings are answered while the resync is still running")
	close(h.release)
	require.ErrorIs(t, stop(), context.Canceled)
}

type slowConnect struct {
	release chan struct{}
}

func (s *slowConnect) Connected(ctx context.Context) {
	select {
	case <-s.release:
	case <-ctx.Done():
	}
}

func (*slowConnect) Event(context.Context, stream.Event) {}

func TestRun_SkipsUndecodableFrames(t *testing.T) {
	dial, _ := serve(t, func(ctx context.Context, conn *websocket.Conn) {
		write(ctx, conn, `not json`, `{"data":{}}`, `{"type":"trade.updated","data":{"status":"settled"}}`)

		_, _, _ = conn.Read(ctx)
	})

	h := &recorder{}
	stop := run(t, dial, h, stream.Options{})

	require.Eventually(t, func() bool { _, events := h.seen(); return len(events) == 1 }, wait, tick)
	require.ErrorIs(t, stop(), context.Canceled)

	_, events := h.seen()
	require.Equal(t, "trade.updated", events[0].Type)
}

func TestRun_StopsWithTheContext(t *testing.T) {
	failing := func(context.Context) (*websocket.Conn, error) { return nil, errors.New("desk is down") }

	h := &recorder{}
	stop := run(t, failing, h, stream.Options{Backoff: time.Hour})

	time.Sleep(50 * time.Millisecond)
	require.ErrorIs(t, stop(), context.Canceled)

	connected, _ := h.seen()
	require.Zero(t, connected)
}
