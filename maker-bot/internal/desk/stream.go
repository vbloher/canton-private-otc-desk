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
	header := http.Header{}
	if token != "" {
		header.Set("Cookie", sessionCookie+"="+token)
	}

	conn, resp, err := websocket.Dial(ctx, c.streamURL, &websocket.DialOptions{HTTPHeader: header, HTTPClient: c.dialer})
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}

	if err == nil {
		return conn, nil
	}

	switch {
	case resp != nil && resp.StatusCode == http.StatusUnauthorized:
		return nil, fmt.Errorf("%w: %w", ErrUnauthorized, err)
	case resp != nil && resp.StatusCode >= http.StatusInternalServerError:
		return nil, fmt.Errorf("%w: %w", ErrUnavailable, err)
	default:
		return nil, fmt.Errorf("websocket.Dial: %w", err)
	}
}
