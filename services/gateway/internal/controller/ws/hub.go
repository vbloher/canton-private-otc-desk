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
	PingInterval = 30 * time.Second

	pongTimeout  = 10 * time.Second
	writeTimeout = 10 * time.Second
	queueSize    = 64
)

type Conn interface {
	Write(ctx context.Context, typ websocket.MessageType, p []byte) error
	Ping(ctx context.Context) error
	Close(code websocket.StatusCode, reason string) error
}

var _ Conn = (*websocket.Conn)(nil)

type Hub struct {
	pingInterval time.Duration

	mu      sync.RWMutex
	parties map[string]map[*client]struct{}
	closed  bool
	wg      sync.WaitGroup
}

func NewHub(pingInterval time.Duration) *Hub {
	if pingInterval <= 0 {
		pingInterval = PingInterval
	}

	return &Hub{
		pingInterval: pingInterval,
		parties:      make(map[string]map[*client]struct{}),
	}
}

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
	closeCode   websocket.StatusCode
	closeReason string
}

func (c *client) shutdown(code websocket.StatusCode, reason string) {
	c.once.Do(func() {
		c.closeCode = code
		c.closeReason = reason

		close(c.stop)
	})
}

func (c *client) run(ctx context.Context, pingInterval time.Duration) (code websocket.StatusCode, reason string) {
	ticker := time.NewTicker(pingInterval)
	defer ticker.Stop()

	for {
		stopCode, stopReason, stop := c.stopped(ctx)
		if stop {
			return stopCode, stopReason
		}

		select {
		case <-c.stop:
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
