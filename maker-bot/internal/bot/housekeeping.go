package bot

import (
	"context"
	"errors"
	"math/big"

	"github.com/vbloher/canton-private-otc-desk/maker-bot/internal/desk"
	"github.com/vbloher/canton-private-otc-desk/maker-bot/internal/strategy"
)

const lowDrips = 10

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
	network, err := b.desk.Network(ctx)
	if err != nil {
		b.log.Warn("network unreadable", "err", err)

		return
	}

	if len(network.Faucet) == 0 {
		return
	}

	balances, err := b.desk.Balances(ctx)
	if err != nil {
		b.log.Warn("balances unreadable", "err", err)

		return
	}

	if !low(network.Faucet, balances) {
		return
	}

	err = b.desk.Faucet(ctx)

	switch {
	case errors.Is(err, desk.ErrTooManyRequests):
		b.log.Debug("the faucet is cooling down")
	case err != nil:
		b.log.Warn("faucet failed", "err", err)
	default:
		b.log.Info("faucet drip requested")
		b.signPending(ctx)
	}
}

func low(drips []desk.Drip, balances map[string]*big.Rat) bool {
	for _, drip := range drips {
		threshold := new(big.Rat).Mul(drip.Amount, big.NewRat(lowDrips, 1))

		available := balances[drip.Symbol]
		if available == nil || available.Cmp(threshold) < 0 {
			return true
		}
	}

	return false
}
