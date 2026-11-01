package bot

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/vbloher/canton-private-otc-desk/maker-bot/internal/config"
	"github.com/vbloher/canton-private-otc-desk/maker-bot/internal/stream"
)

type Signer interface {
	Sign(message []byte) []byte
}

type Bot struct {
	desk   Desk
	signer Signer
	config config.Config
	log    *slog.Logger
	now    func() time.Time

	mu          sync.Mutex
	sent        map[uuid.UUID]bool
	refused     map[uuid.UUID]bool
	faucetAfter time.Time
}

var _ stream.Handler = (*Bot)(nil)

func New(d Desk, s Signer, c config.Config, log *slog.Logger, now func() time.Time) *Bot {
	return &Bot{
		desk:    d,
		signer:  s,
		config:  c,
		log:     log,
		now:     now,
		sent:    map[uuid.UUID]bool{},
		refused: map[uuid.UUID]bool{},
	}
}

func (b *Bot) Connected(ctx context.Context) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.refresh(ctx, true)
}

type change struct {
	RFQID  uuid.UUID `json:"rfqId"`
	Status string    `json:"status"`
}

func (b *Bot) Event(ctx context.Context, ev stream.Event) {
	b.mu.Lock()
	defer b.mu.Unlock()

	switch ev.Type {
	case "rfq.created", "rfq.updated":
		b.refresh(ctx, false)
	case "quote.updated":
		c, ok := b.change(ev)
		if ok && c.Status == "expired" {
			b.refresh(ctx, false)
		}
	case "trade.updated":
		c, ok := b.change(ev)
		if ok {
			b.traded(ctx, c)
		}
	case "sign.requested":
		b.signPending(ctx)
	case "wallet.updated":
		b.acceptTransfers(ctx)
	}
}

func (b *Bot) Housekeeping(ctx context.Context) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.signPending(ctx)
	b.acceptTransfers(ctx)
	b.topUp(ctx)
}

func (b *Bot) change(ev stream.Event) (change, bool) {
	var c change

	err := json.Unmarshal(ev.Data, &c)
	if err != nil {
		b.log.Warn("event skipped", "type", ev.Type, "err", err)

		return change{}, false
	}

	return c, true
}

func (b *Bot) traded(ctx context.Context, c change) {
	switch c.Status {
	case "settled":
		b.log.Info("trade settled", "rfq", c.RFQID)
	case "failed":
		b.log.Info("trade failed, quoting again", "rfq", c.RFQID)
		b.requote(ctx, c.RFQID)
	}
}
