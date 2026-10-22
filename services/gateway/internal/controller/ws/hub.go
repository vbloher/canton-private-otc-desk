// Package ws serves the events WebSocket: the hub fans events out to the sockets of the addressed party,
// the handler upgrades authenticated requests and registers their sockets.
package ws

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/rs/zerolog/log"
)

const (
	// PingInterval is how often the hub pings every socket.
	PingInterval = 30 * time.Second

	// pongTimeout is how long a socket may take to answer a ping before it counts as dead.
	pongTimeout = 10 * time.Second
	// writeTimeout bounds one frame write.
	writeTimeout = 10 * time.Second
	// queueSize is how many events may wait for one socket; a socket that falls further behind is dropped.
	queueSize = 64
)

// Conn is the part of a WebSocket the hub needs; *websocket.Conn implements it.
type Conn interface {
	Write(ctx context.Context, typ websocket.MessageType, p []byte) error
	Ping(ctx context.Context) error
	Close(code websocket.StatusCode, reason string) error
}

var _ Conn = (*websocket.Conn)(nil)

// Hub delivers each event to every socket of the party it is addressed to.
//
// Every socket has its own goroutine, queue and ping timer, so the single goroutine feeding Deliver
// never waits for a client: a client that cannot keep up is closed and expected to reconnect
// (delivery is at-most-once and the UI re-fetches over REST anyway).
type Hub struct {
	pingInterval time.Duration

	mu      sync.RWMutex
	parties map[string]map[*client]struct{}
	closed  bool
	wg      sync.WaitGroup // one per running socket goroutine
}

// NewHub pings sockets every pingInterval (PingInterval when it is not positive).
func NewHub(pingInterval time.Duration) *Hub {
	if pingInterval <= 0 {
		pingInterval = PingInterval
	}

	return &Hub{
		pingInterval: pingInterval,
		parties:      make(map[string]map[*client]struct{}),
	}
}

// Register adds the socket to the party and starts serving it.
// ctx is the socket's lifetime: once it is done the socket is closed and forgotten, and it bounds every write and ping.
// The returned function does the same on demand; it is safe to call more than once.
// A socket registered after Close is closed right away.
func (h *Hub) Register(ctx context.Context, partyID string, conn Conn) func() {
	c := &client{
		partyID: partyID,
		conn:    conn,
		send:    make(chan []byte, queueSize),
		stop:    make(chan struct{}),
	}

	h.mu.Lock()

	if h.closed {
		h.mu.Unlock()

		c.closeConn(websocket.StatusGoingAway, "server shutting down")

		return func() {}
	}

	sockets := h.parties[partyID]
	if sockets == nil {
		sockets = make(map[*client]struct{})
		h.parties[partyID] = sockets
	}

	sockets[c] = struct{}{}

	h.wg.Add(1)
	h.mu.Unlock()

	go h.serve(ctx, c)

	return func() { c.shutdown(websocket.StatusNormalClosure, "") }
}

// Deliver queues the payload for every socket of the party. It never blocks and the payload must not be
// modified afterwards: all sockets share it.
func (h *Hub) Deliver(partyID string, payload []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for c := range h.parties[partyID] {
		select {
		case c.send <- payload:
		default:
			c.shutdown(websocket.StatusTryAgainLater, "too slow")
		}
	}
}

// Close says goodbye to every socket and waits until they are closed.
func (h *Hub) Close() {
	h.mu.Lock()

	h.closed = true

	for _, sockets := range h.parties {
		for c := range sockets {
			c.shutdown(websocket.StatusGoingAway, "server shutting down")
		}
	}

	h.mu.Unlock()

	h.wg.Wait()
}

func (h *Hub) serve(ctx context.Context, c *client) {
	defer h.wg.Done()

	code, reason := c.run(ctx, h.pingInterval)

	h.remove(c)

	if code != websocket.StatusNormalClosure {
		log.Info().Str("reason", reason).Msg("ws: dropping socket")
	}

	c.closeConn(code, reason)
}

func (h *Hub) remove(c *client) {
	h.mu.Lock()
	defer h.mu.Unlock()

	sockets := h.parties[c.partyID]

	delete(sockets, c)

	if len(sockets) == 0 {
		delete(h.parties, c.partyID)
	}
}

type client struct {
	partyID string
	conn    Conn
	send    chan []byte
	stop    chan struct{}

	once        sync.Once
	closeCode   websocket.StatusCode // why the socket is closed; written before stop is closed
	closeReason string
}

// shutdown asks the socket goroutine to close the socket. The first reason wins.
func (c *client) shutdown(code websocket.StatusCode, reason string) {
	c.once.Do(func() {
		c.closeCode = code
		c.closeReason = reason

		close(c.stop)
	})
}

// run writes queued events and pings until the socket is asked to stop, its context is done or it turns out
// to be dead. It returns how the socket is to be closed.
func (c *client) run(ctx context.Context, pingInterval time.Duration) (code websocket.StatusCode, reason string) {
	ticker := time.NewTicker(pingInterval)
	defer ticker.Stop()

	for {
		// Checked before anything else, so queued events are not written to a socket after it was let go.
		stopCode, stopReason, stop := c.stopped(ctx)
		if stop {
			return stopCode, stopReason
		}

		select {
		case <-c.stop: // handled by stopped on the next round
		case <-ctx.Done():
		case payload := <-c.send:
			err := c.write(ctx, payload)
			if err != nil {
				log.Debug().Err(err).Msg("ws: write")

				return websocket.StatusGoingAway, "write failed"
			}
		case <-ticker.C:
			err := c.ping(ctx)
			if err != nil {
				log.Debug().Err(err).Msg("ws: ping")

				return websocket.StatusGoingAway, "ping failed"
			}
		}
	}
}

// stopped tells whether the socket was asked to stop or its context is done, and how to close it then.
func (c *client) stopped(ctx context.Context) (code websocket.StatusCode, reason string, stopped bool) {
	select {
	case <-c.stop:
		return c.closeCode, c.closeReason, true
	case <-ctx.Done():
		return websocket.StatusNormalClosure, "", true
	default:
		return 0, "", false
	}
}

func (c *client) write(ctx context.Context, payload []byte) error {
	ctx, cancel := context.WithTimeout(ctx, writeTimeout)
	defer cancel()

	err := c.conn.Write(ctx, websocket.MessageText, payload)
	if err != nil {
		return fmt.Errorf("conn.Write: %w", err)
	}

	return nil
}

func (c *client) ping(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, pongTimeout)
	defer cancel()

	err := c.conn.Ping(ctx)
	if err != nil {
		return fmt.Errorf("conn.Ping: %w", err)
	}

	return nil
}

func (c *client) closeConn(code websocket.StatusCode, reason string) {
	err := c.conn.Close(code, reason)
	if err != nil {
		log.Debug().Err(err).Msg("ws: close")
	}
}
