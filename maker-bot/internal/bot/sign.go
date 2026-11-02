package bot

import (
	"context"
	"errors"
	"sync"

	"github.com/google/uuid"

	"github.com/vbloher/canton-private-otc-desk/maker-bot/internal/desk"
)

const purposeTransferOut = "transfer-out"

func (b *Bot) signPending(ctx context.Context) {
	actions, err := b.desk.PendingActions(ctx)
	if err != nil {
		b.log.Warn("pending actions unreadable", "err", err)

		return
	}

	b.forgetFinished(actions)

	var due []desk.Action

	for _, a := range actions {
		if a.Purpose == purposeTransferOut {
			b.refuse(a)

			continue
		}

		if b.sent[a.ID] {
			continue
		}

		b.sent[a.ID] = true
		due = append(due, a)
	}

	retry := make([]bool, len(due))

	var wg sync.WaitGroup

	for i, a := range due {
		wg.Go(func() { retry[i] = b.execute(ctx, a) })
	}

	wg.Wait()

	for i, a := range due {
		if retry[i] {
			delete(b.sent, a.ID)
		}
	}
}

func (b *Bot) execute(ctx context.Context, a desk.Action) bool {
	err := b.desk.Execute(ctx, a.ID, b.signer.Sign(a.Hash))

	switch {
	case err == nil:
		b.log.Info("signed", "action", a.ID, "purpose", a.Purpose, "description", a.Description)
	case errors.Is(err, desk.ErrConflict), errors.Is(err, desk.ErrNotFound), errors.Is(err, desk.ErrGone):
		b.log.Info("action is gone", "action", a.ID, "err", err)
	default:
		b.log.Warn("signing failed, will retry", "action", a.ID, "err", err)

		return true
	}

	return false
}

func (b *Bot) refuse(a desk.Action) {
	if b.refused[a.ID] {
		return
	}

	b.refused[a.ID] = true
	b.log.Warn("refusing to sign a transfer-out", "action", a.ID, "description", a.Description)
}

func (b *Bot) forgetFinished(pending []desk.Action) {
	listed := make(map[uuid.UUID]bool, len(pending))
	for _, a := range pending {
		listed[a.ID] = true
	}

	for id := range b.sent {
		if !listed[id] {
			delete(b.sent, id)
		}
	}

	for id := range b.refused {
		if !listed[id] {
			delete(b.refused, id)
		}
	}
}
