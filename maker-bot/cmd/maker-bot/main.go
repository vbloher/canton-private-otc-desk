package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/vbloher/canton-private-otc-desk/maker-bot/internal/bot"
	"github.com/vbloher/canton-private-otc-desk/maker-bot/internal/config"
	"github.com/vbloher/canton-private-otc-desk/maker-bot/internal/desk"
	"github.com/vbloher/canton-private-otc-desk/maker-bot/internal/identity"
)

const (
	requestTimeout = 30 * time.Second
	retryDelay     = 5 * time.Second
)

func main() {
	err := run()
	if err != nil {
		slog.Error("maker bot stopped", "err", err)
		os.Exit(1)
	}
}

func run() error {
	c, err := config.Load()
	if err != nil {
		return fmt.Errorf("config.Load: %w", err)
	}

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: c.LogLevel}))
	slog.SetDefault(log)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	client, err := desk.New(c.DeskURL, &http.Client{Timeout: requestTimeout}, log)
	if err != nil {
		return fmt.Errorf("desk.New: %w", err)
	}

	var id identity.Identity

	err = bot.Retry(ctx, retryDelay, log, func(ctx context.Context) error {
		var err error

		id, err = bot.Enroll(ctx, client, c.StateFile, c.MakerName, log)
		if err != nil {
			return fmt.Errorf("bot.Enroll: %w", err)
		}

		return nil
	})
	if err != nil {
		return stopped(err)
	}

	log.Info("maker bot started", "party", id.PartyID, "hint", id.Hint, "desk", c.DeskURL, "spread_bps", c.SpreadBps)

	return stopped(bot.New(client, id, c, log, time.Now).Run(ctx, client.Dial))
}

func stopped(err error) error {
	if errors.Is(err, context.Canceled) {
		slog.Info("maker bot stopped")

		return nil
	}

	return err
}
