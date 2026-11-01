package bot

import (
	"context"
	"errors"
	"math/big"
	"slices"
	"time"

	"github.com/vbloher/canton-private-otc-desk/maker-bot/internal/desk"
	"github.com/vbloher/canton-private-otc-desk/maker-bot/internal/strategy"
)

const faucetPause = time.Hour

func (b *Bot) acceptTransfers(ctx context.Context) {
	transfers, err := b.desk.IncomingTransfers(ctx)
	if err != nil {
		b.log.Warn("incoming transfers unreadable", "err", err)

		return
	}

	accepted := false

	for _, t := range transfers {
		err := b.desk.AcceptTransfer(ctx, t.ContractID)
		if err != nil {
			b.log.Warn("transfer not accepted", "contract", t.ContractID, "err", err)

			continue
		}

		accepted = true

		b.log.Info("transfer accepted", "contract", t.ContractID, "sender", t.Sender, "amount", strategy.Format(t.Amount),
			"symbol", t.Symbol)
	}

	if accepted {
		b.signPending(ctx)
	}
}

func (b *Bot) topUp(ctx context.Context) {
	if b.faucetDone || (b.faucetAsked == nil && b.now().Before(b.faucetAfter)) {
		return
	}

	short, ok := b.shortOfDrips(ctx)
	if !ok || !b.dripSettled(short) {
		return
	}

	if len(short) == 0 || b.now().Before(b.faucetAfter) {
		return
	}

	b.askFaucet(ctx, short)
}

func (b *Bot) askFaucet(ctx context.Context, short []string) {
	b.faucetAfter = b.now().Add(faucetPause)

	err := b.desk.Faucet(ctx)

	switch {
	case errors.Is(err, desk.ErrTooManyRequests):
		b.log.Debug("the faucet is cooling down")

		return
	case err != nil:
		b.log.Warn("faucet failed", "err", err)
	default:
		b.log.Info("faucet drip requested", "symbols", short)
		b.signPending(ctx)
	}

	b.faucetAsked = short
}

func (b *Bot) shortOfDrips(ctx context.Context) ([]string, bool) {
	network, err := b.desk.Network(ctx)
	if err != nil {
		b.log.Warn("network unreadable", "err", err)

		return nil, false
	}

	if len(network.Faucet) == 0 {
		return nil, false
	}

	balances, err := b.desk.Balances(ctx)
	if err != nil {
		b.log.Warn("balances unreadable", "err", err)

		return nil, false
	}

	return belowADrip(network.Faucet, balances), true
}

func (b *Bot) dripSettled(short []string) bool {
	if b.faucetAsked == nil {
		return true
	}

	missing := slices.DeleteFunc(slices.Clone(b.faucetAsked), func(symbol string) bool {
		return !slices.Contains(short, symbol)
	})
	if len(missing) == 0 {
		b.faucetAsked = nil

		return true
	}

	if b.now().Before(b.faucetAfter) {
		return false
	}

	b.faucetDone = true
	b.log.Warn("the faucet did not refill, no more drips until a restart", "symbols", missing)

	return false
}

func belowADrip(drips []desk.Drip, balances map[string]*big.Rat) []string {
	var out []string

	for _, drip := range drips {
		available := balances[drip.Symbol]
		if available == nil || available.Cmp(drip.Amount) < 0 {
			out = append(out, drip.Symbol)
		}
	}

	return out
}
