package desk

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/coder/websocket"
)

func (c *Client) Dial(ctx context.Context) (*websocket.Conn, error) {
	token := c.session.current()

	conn, err := c.dial(ctx, token)
	if !errors.Is(err, ErrUnauthorized) {
		return conn, err
	}

	err = c.session.renewFrom(ctx, token)
	if err != nil {
		return nil, fmt.Errorf("sign in: %w", err)
	}

	return c.dial(ctx, c.session.current())
}

func (c *Client) dial(ctx context.Context, token string) (*websocket.Conn, error) {
	if c.dialTimeout > 0 {
		var cancel context.CancelFunc

		ctx, cancel = context.WithTimeout(ctx, c.dialTimeout)
		defer cancel()
	}

	header := http.Header{}
	if token != "" {
		header.Set("Cookie", sessionCookie+"="+token)
	}

	conn, resp, err := websocket.Dial(ctx, c.streamURL, &websocket.DialOptions{HTTPHeader: header, HTTPClient: c.dialer})
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}

	if err != nil {
		return nil, handshakeError(resp, err)
	}

	return conn, nil
}

func handshakeError(resp *http.Response, err error) error {
	switch {
	case resp != nil && resp.StatusCode == http.StatusUnauthorized:
		return fmt.Errorf("%w: %w", ErrUnauthorized, err)
	case resp != nil && resp.StatusCode >= http.StatusInternalServerError:
		return fmt.Errorf("%w: %w", ErrUnavailable, err)
	default:
		return fmt.Errorf("websocket.Dial: %w", err)
	}
}
