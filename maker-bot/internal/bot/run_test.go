package bot_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/maker-bot/internal/bot"
	"github.com/vbloher/canton-private-otc-desk/maker-bot/internal/desk"
)

func Test_Run_TurnsMakerModeOnFirst(t *testing.T) {
	d := &fakeDesk{}
	b, _, _ := newBot(t, d)

	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()

	failing := func(context.Context) (*websocket.Conn, error) { return nil, errors.New("desk is down") }

	err := b.Run(ctx, failing)
	require.ErrorIs(t, err, context.DeadlineExceeded)

	calls := d.callNames()
	require.NotEmpty(t, calls)
	require.Equal(t, "SetMakerMode", calls[0])
	require.Contains(t, calls, "PendingActions", "housekeeping runs once before the stream")
	require.Equal(t, []bool{true}, d.makerMode)
}

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

func Test_Retry_RetriesTransientErrors(t *testing.T) {
	attempts := 0

	err := bot.Retry(t.Context(), time.Millisecond, quiet, func(context.Context) error {
		attempts++
		if attempts < 3 {
			return fmt.Errorf("register start: %w", desk.ErrUnavailable)
		}

		return nil
	})
	require.NoError(t, err)
	require.Equal(t, 3, attempts)
}

func Test_Retry_StopsOnAPermanentError(t *testing.T) {
	attempts := 0

	err := bot.Retry(t.Context(), time.Millisecond, quiet, func(context.Context) error {
		attempts++

		return bot.ErrUnknownParty
	})
	require.ErrorIs(t, err, bot.ErrUnknownParty)
	require.Equal(t, 1, attempts)
}

func Test_Retry_StopsWithTheContext(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	err := bot.Retry(ctx, time.Hour, quiet, func(context.Context) error { return desk.ErrUnavailable })
	require.ErrorIs(t, err, context.Canceled)
}
