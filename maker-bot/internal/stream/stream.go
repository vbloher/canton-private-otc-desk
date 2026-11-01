package stream

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/coder/websocket"
)

const (
	defaultBackoff      = 5 * time.Second
	defaultPingInterval = 30 * time.Second
	defaultPingTimeout  = 10 * time.Second

	queueSize  = 256
	frameShown = 120
)

type Event struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

type Handler interface {
	Connected(ctx context.Context)
	Event(ctx context.Context, ev Event)
}

type Options struct {
	Backoff      time.Duration
	PingInterval time.Duration
	PingTimeout  time.Duration
}

type Dial func(ctx context.Context) (*websocket.Conn, error)

func Run(ctx context.Context, dial Dial, h Handler, opts Options, log *slog.Logger) error {
	opts = opts.withDefaults()

	for {
		err := connect(ctx, dial, h, opts, log)
		if ctx.Err() != nil {
			return fmt.Errorf("stream: %w", ctx.Err())
		}

		log.Warn("stream closed", "err", err, "retry_in", opts.Backoff.String())

		select {
		case <-ctx.Done():
			return fmt.Errorf("stream: %w", ctx.Err())
		case <-time.After(opts.Backoff):
		}
	}
}

func (o Options) withDefaults() Options {
	if o.Backoff <= 0 {
		o.Backoff = defaultBackoff
	}

	if o.PingInterval <= 0 {
		o.PingInterval = defaultPingInterval
	}

	if o.PingTimeout <= 0 {
		o.PingTimeout = defaultPingTimeout
	}

	return o
}

func connect(ctx context.Context, dial Dial, h Handler, opts Options, log *slog.Logger) error {
	conn, err := dial(ctx)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}

	defer func() { _ = conn.CloseNow() }()

	log.Info("stream connected")

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	events := make(chan Event, queueSize)
	readErr := make(chan error, 1)

	go func() { readErr <- read(ctx, conn, events, log) }()
	go keepAlive(ctx, conn, opts, log)

	h.Connected(ctx)

	for ev := range events {
		h.Event(ctx, ev)
	}

	return <-readErr
}

func read(ctx context.Context, conn *websocket.Conn, events chan<- Event, log *slog.Logger) error {
	defer close(events)

	for {
		typ, frame, err := conn.Read(ctx)
		if err != nil {
			return fmt.Errorf("conn.Read: %w", err)
		}

		if typ != websocket.MessageText {
			continue
		}

		var ev Event

		err = json.Unmarshal(frame, &ev)
		if err != nil || ev.Type == "" {
			log.Warn("undecodable frame skipped", "frame", string(frame[:min(len(frame), frameShown)]))

			continue
		}

		select {
		case events <- ev:
		case <-ctx.Done():
			return fmt.Errorf("read: %w", ctx.Err())
		}
	}
}

func keepAlive(ctx context.Context, conn *websocket.Conn, opts Options, log *slog.Logger) {
	ticker := time.NewTicker(opts.PingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			pingCtx, cancel := context.WithTimeout(ctx, opts.PingTimeout)
			err := conn.Ping(pingCtx)

			cancel()

			if err != nil && ctx.Err() == nil {
				log.Warn("stream ping failed, dropping the connection", "err", err)

				_ = conn.CloseNow()

				return
			}
		}
	}
}
