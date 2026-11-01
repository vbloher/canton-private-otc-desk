package bot

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/vbloher/canton-private-otc-desk/maker-bot/internal/desk"
	"github.com/vbloher/canton-private-otc-desk/maker-bot/internal/stream"
)

const retryDelay = 5 * time.Second

func (b *Bot) Run(ctx context.Context, dial stream.Dial) error {
	err := Retry(ctx, retryDelay, b.log, func(ctx context.Context) error { return b.desk.SetMakerMode(ctx, true) })
	if err != nil {
		return fmt.Errorf("maker mode: %w", err)
	}

	b.Housekeeping(ctx)

	ctx, cancel := context.WithCancel(ctx)

	var wg sync.WaitGroup

	wg.Go(func() { b.keepHouse(ctx) })

	defer func() {
		cancel()
		wg.Wait()
	}()

	err = stream.Run(ctx, dial, b, stream.Options{}, b.log)
	if err != nil {
		return fmt.Errorf("stream.Run: %w", err)
	}

	return nil
}

func (b *Bot) keepHouse(ctx context.Context) {
	ticker := time.NewTicker(b.config.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			b.Housekeeping(ctx)
		}
	}
}

func Retry(ctx context.Context, every time.Duration, log *slog.Logger, fn func(ctx context.Context) error) error {
	for {
		err := fn(ctx)
		if err == nil || !desk.Transient(err) {
			return err
		}

		log.Warn("the desk is unreachable, retrying", "err", err, "retry_in", every.String())

		select {
		case <-ctx.Done():
			return fmt.Errorf("retry: %w", ctx.Err())
		case <-time.After(every):
		}
	}
}
