package ws_test

import (
	"context"
	"errors"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/pkg/presence"
	"github.com/vbloher/canton-private-otc-desk/services/gateway/internal/controller/ws"
)

const (
	wait = 5 * time.Second
	tick = 5 * time.Millisecond

	quiet = time.Hour
)

type frame struct {
	typ     websocket.MessageType
	payload []byte
}

type fakeConn struct {
	writes chan frame
	closes chan websocket.StatusCode
	pings  atomic.Int64

	pingErr  error
	writeErr error
	gate     chan struct{}
}

func newFakeConn() *fakeConn {
	return &fakeConn{
		writes: make(chan frame, 1024),
		closes: make(chan websocket.StatusCode, 4),
	}
}

func (f *fakeConn) Write(ctx context.Context, typ websocket.MessageType, p []byte) error {
	if f.gate != nil {
		select {
		case <-f.gate:
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	if f.writeErr != nil {
		return f.writeErr
	}

	f.writes <- frame{typ: typ, payload: p}

	return nil
}

func (f *fakeConn) Ping(context.Context) error {
	f.pings.Add(1)

	return f.pingErr
}

func (f *fakeConn) Close(code websocket.StatusCode, _ string) error {
	select {
	case f.closes <- code:
	default:
	}

	return nil
}

func (f *fakeConn) next(t *testing.T) []byte {
	t.Helper()

	select {
	case got := <-f.writes:
		require.Equal(t, websocket.MessageText, got.typ, "events travel as text frames")

		return got.payload
	case <-time.After(wait):
		require.FailNow(t, "no frame arrived")

		return nil
	}
}

func (f *fakeConn) closedWith(t *testing.T) websocket.StatusCode {
	t.Helper()

	select {
	case code := <-f.closes:
		return code
	case <-time.After(wait):
		require.FailNow(t, "the socket was not closed")

		return 0
	}
}

type fakePresence struct {
	mu     sync.Mutex
	marks  [][]string
	ttls   []time.Duration
	clears []string
	online map[string]bool
	err    error

	clearEntered chan struct{}
	clearGate    chan struct{}
}

func (f *fakePresence) MarkOnline(_ context.Context, partyIDs []string, ttl time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.marks = append(f.marks, slices.Sorted(slices.Values(partyIDs)))
	f.ttls = append(f.ttls, ttl)

	if f.online == nil {
		f.online = map[string]bool{}
	}

	for _, partyID := range partyIDs {
		f.online[partyID] = true
	}

	return f.err
}

func (f *fakePresence) ClearOnline(_ context.Context, partyID string) error {
	if f.clearEntered != nil {
		f.clearEntered <- struct{}{}
	}

	if f.clearGate != nil {
		<-f.clearGate
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	f.clears = append(f.clears, partyID)
	delete(f.online, partyID)

	return f.err
}

func (f *fakePresence) isOnline(party string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.online[party]
}

func (f *fakePresence) marked(parties ...string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()

	for i, mark := range f.marks {
		if slices.Equal(mark, parties) && f.ttls[i] == presence.TTL {
			return true
		}
	}

	return false
}

func (f *fakePresence) cleared(party string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()

	return slices.Contains(f.clears, party)
}

func newHub(t *testing.T, pingInterval time.Duration) *ws.Hub {
	t.Helper()

	return newHubWithPresence(t, pingInterval, quiet, &fakePresence{})
}

func newHubWithPresence(t *testing.T, pingInterval, refresh time.Duration, p ws.Presence) *ws.Hub {
	t.Helper()

	hub := ws.NewHub(pingInterval, refresh, p)
	t.Cleanup(hub.Close)

	return hub
}

func Test_Hub_MarksAPartyOnlineWhenItConnects(t *testing.T) {
	p := &fakePresence{}
	hub := newHubWithPresence(t, quiet, quiet, p)

	hub.Register(t.Context(), "alice::1220", newFakeConn())

	require.Eventually(t, func() bool { return p.marked("alice::1220") }, wait, tick)
}

func Test_Hub_KeepsAPartyOnlineWhileASocketLives(t *testing.T) {
	p := &fakePresence{}
	hub := newHubWithPresence(t, quiet, quiet, p)

	closeFirst := hub.Register(t.Context(), "alice::1220", newFakeConn())
	hub.Register(t.Context(), "alice::1220", newFakeConn())

	closeFirst()

	require.Eventually(t, func() bool { return hub.Len() == 1 }, wait, tick)
	require.Never(t, func() bool { return p.cleared("alice::1220") }, 50*time.Millisecond, tick)
}

func Test_Hub_ClearsAPartyWhenItsLastSocketCloses(t *testing.T) {
	p := &fakePresence{}
	hub := newHubWithPresence(t, quiet, quiet, p)

	closeIt := hub.Register(t.Context(), "bob::1220", newFakeConn())
	closeIt()

	require.Eventually(t, func() bool { return p.cleared("bob::1220") }, wait, tick)
}

func Test_Hub_StaysOnlineWhenAPartyReconnectsDuringTheTeardown(t *testing.T) {
	p := &fakePresence{clearEntered: make(chan struct{}, 1), clearGate: make(chan struct{})}
	hub := newHubWithPresence(t, quiet, quiet, p)

	closeOld := hub.Register(t.Context(), "alice::1220", newFakeConn())
	require.True(t, p.isOnline("alice::1220"))

	closeOld()

	select {
	case <-p.clearEntered:
	case <-time.After(wait):
		require.FailNow(t, "the old socket's teardown never cleared the party")
	}

	hub.Register(t.Context(), "alice::1220", newFakeConn())
	close(p.clearGate)

	require.Eventually(t, func() bool { return p.cleared("alice::1220") }, wait, tick)
	require.Eventually(t, func() bool { return p.isOnline("alice::1220") }, wait, tick,
		"a party with a live socket is online even when the old socket's clear lands after the new mark")
}

func Test_Hub_RefreshesOnlineParties(t *testing.T) {
	p := &fakePresence{}
	hub := newHubWithPresence(t, quiet, 10*time.Millisecond, p)

	hub.Register(t.Context(), "alice::1220", newFakeConn())
	hub.Register(t.Context(), "bob::1220", newFakeConn())

	require.Eventually(t, func() bool { return p.marked("alice::1220", "bob::1220") }, wait, tick)
}

func Test_Hub_PresenceErrorsKeepSockets(t *testing.T) {
	p := &fakePresence{err: errors.New("redis is down")}
	hub := newHubWithPresence(t, quiet, 10*time.Millisecond, p)

	conn := newFakeConn()
	hub.Register(t.Context(), "alice::1220", conn)

	event := []byte(`{"type":"wallet.updated","data":{}}`)
	hub.Deliver("alice::1220", event)

	require.Equal(t, event, conn.next(t))
}

func Test_Hub_DeliversToPartyOnly(t *testing.T) {
	hub := newHub(t, quiet)

	alice1, alice2, bob := newFakeConn(), newFakeConn(), newFakeConn()
	hub.Register(t.Context(), "alice::1220", alice1)
	hub.Register(t.Context(), "alice::1220", alice2)
	hub.Register(t.Context(), "bob::1220", bob)

	event := []byte(`{"type":"rfq.created","data":{"rfqId":"7"}}`)

	hub.Deliver("alice::1220", event)
	hub.Deliver("nobody::1220", []byte(`{"type":"wallet.updated","data":{}}`))

	require.Equal(t, event, alice1.next(t), "every socket of the party gets the payload unchanged")
	require.Equal(t, event, alice2.next(t))

	bobEvent := []byte(`{"type":"trade.updated","data":{"tradeId":"1","rfqId":"7","status":"settled"}}`)
	hub.Deliver("bob::1220", bobEvent)
	require.Equal(t, bobEvent, bob.next(t))

	followUp := []byte(`{"type":"wallet.updated","data":{}}`)
	hub.Deliver("alice::1220", followUp)
	require.Equal(t, followUp, alice1.next(t))
	require.Equal(t, followUp, alice2.next(t))
}

func Test_Hub_UnregisterStopsDelivery(t *testing.T) {
	hub := newHub(t, quiet)

	leaving, staying := newFakeConn(), newFakeConn()
	unregister := hub.Register(t.Context(), "alice::1220", leaving)
	hub.Register(t.Context(), "alice::1220", staying)

	unregister()
	unregister()

	require.Equal(t, websocket.StatusNormalClosure, leaving.closedWith(t), "the hub closes the socket it lets go of")

	hub.Deliver("alice::1220", []byte("after"))
	require.Equal(t, []byte("after"), staying.next(t))
	require.Empty(t, leaving.writes, "an unregistered socket gets nothing")
}

func Test_Hub_PingsAndDropsDeadSocket(t *testing.T) {
	hub := newHub(t, 10*time.Millisecond)

	healthy, dead := newFakeConn(), newFakeConn()
	dead.pingErr = errors.New("no pong")

	hub.Register(t.Context(), "alice::1220", healthy)
	hub.Register(t.Context(), "alice::1220", dead)

	require.Eventually(t, func() bool { return healthy.pings.Load() >= 3 }, wait, tick, "live sockets are pinged repeatedly")

	require.Equal(t, websocket.StatusGoingAway, dead.closedWith(t), "a socket that misses its pong is dropped")
	require.Equal(t, int64(1), dead.pings.Load(), "...at the first failed ping")

	hub.Deliver("alice::1220", []byte("still here"))
	require.Equal(t, []byte("still here"), healthy.next(t), "the healthy socket of the same party is unaffected")
	require.Empty(t, dead.writes)
}

func Test_Hub_DropsSocketOnWriteError(t *testing.T) {
	hub := newHub(t, quiet)

	broken := newFakeConn()
	broken.writeErr = errors.New("broken pipe")

	hub.Register(t.Context(), "alice::1220", broken)
	hub.Deliver("alice::1220", []byte("event"))

	require.Equal(t, websocket.StatusGoingAway, broken.closedWith(t))
}

func Test_Hub_DropsSlowSocketWithoutBlocking(t *testing.T) {
	hub := newHub(t, quiet)

	slow := newFakeConn()
	slow.gate = make(chan struct{})

	fast := newFakeConn()

	hub.Register(t.Context(), "slow::1220", slow)
	hub.Register(t.Context(), "fast::1220", fast)

	delivered := make(chan struct{})

	go func() {
		defer close(delivered)

		for range 1000 {
			hub.Deliver("slow::1220", []byte("event"))
		}
	}()

	select {
	case <-delivered:
	case <-time.After(wait):
		require.FailNow(t, "Deliver blocked on a socket that does not read")
	}

	hub.Deliver("fast::1220", []byte("unaffected"))
	require.Equal(t, []byte("unaffected"), fast.next(t))

	close(slow.gate)

	require.Equal(t, websocket.StatusTryAgainLater, slow.closedWith(t), "the client reconnects and re-fetches over REST")
	require.Empty(t, fast.closes, "other sockets are not touched")
}

func Test_Hub_CloseSaysGoodbyeToEverySocket(t *testing.T) {
	hub := ws.NewHub(quiet, quiet, &fakePresence{})

	a, b := newFakeConn(), newFakeConn()
	hub.Register(t.Context(), "alice::1220", a)
	hub.Register(t.Context(), "bob::1220", b)

	hub.Close()

	require.Equal(t, websocket.StatusGoingAway, a.closedWith(t))
	require.Equal(t, websocket.StatusGoingAway, b.closedWith(t))

	late := newFakeConn()
	unregister := hub.Register(t.Context(), "alice::1220", late)

	require.Equal(t, websocket.StatusGoingAway, late.closedWith(t), "a socket arriving after shutdown is turned away")

	unregister()
	hub.Deliver("alice::1220", []byte("late event"))
	hub.Close()

	require.Empty(t, late.writes)
}
