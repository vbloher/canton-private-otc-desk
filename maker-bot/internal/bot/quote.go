package bot

import (
	"context"
	"errors"
	"math/big"
	"time"

	"github.com/google/uuid"

	"github.com/vbloher/canton-private-otc-desk/maker-bot/internal/desk"
	"github.com/vbloher/canton-private-otc-desk/maker-bot/internal/strategy"
)

const minQuoteTTL = 10 * time.Second

func (b *Bot) refresh(ctx context.Context, retryFailed bool) {
	incoming, err := b.desk.IncomingRFQs(ctx)
	if err != nil {
		b.log.Warn("incoming rfqs unreadable", "err", err)

		return
	}

	for _, in := range incoming {
		if in.RFQ.Open && quotable(in.MyQuote, retryFailed) {
			b.quote(ctx, in.RFQ)
		}
	}
}

func (b *Bot) requote(ctx context.Context, rfqID uuid.UUID) {
	incoming, err := b.desk.IncomingRFQs(ctx)
	if err != nil {
		b.log.Warn("incoming rfqs unreadable", "err", err)

		return
	}

	for _, in := range incoming {
		if in.RFQ.ID == rfqID && in.RFQ.Open && quotable(in.MyQuote, true) {
			b.quote(ctx, in.RFQ)
		}
	}
}

func quotable(mine *desk.Quote, retryFailed bool) bool {
	return mine == nil || mine.Status == "expired" || (retryFailed && mine.Status == "failed")
}

func (b *Bot) quote(ctx context.Context, rfq desk.RFQ) {
	mid, err := strategy.Mid(b.config.Prices, rfq.Base, rfq.Quote)
	if err != nil {
		b.skip(rfq, err.Error())

		return
	}

	price, err := strategy.Price(mid, rfq.TakerBuys, b.config.SpreadBps)
	if err != nil {
		b.skip(rfq, err.Error())

		return
	}

	ttl := min(b.config.QuoteTTL, rfq.Deadline.Sub(b.now())).Truncate(time.Second)
	if ttl < minQuoteTTL {
		b.skip(rfq, "the rfq closes in less than "+minQuoteTTL.String())

		return
	}

	if !b.affordable(ctx, rfq, price) {
		return
	}

	q, err := b.desk.SubmitQuote(ctx, rfq.ID, strategy.Format(price), ttl)

	switch {
	case errors.Is(err, desk.ErrConflict), errors.Is(err, desk.ErrNotFound):
		b.skip(rfq, err.Error())
	case err != nil:
		b.log.Warn("quote failed", "rfq", rfq.ID, "err", err)
	default:
		b.log.Info("quoted", "rfq", rfq.ID, "quote", q.ID, "taker_buys", rfq.TakerBuys, "qty", strategy.Format(rfq.Qty),
			"base", rfq.Base, "quote_asset", rfq.Quote, "price", strategy.Format(price), "ttl", ttl.String())
		b.signPending(ctx)
	}
}

func (b *Bot) affordable(ctx context.Context, rfq desk.RFQ, price *big.Rat) bool {
	balances, err := b.desk.Balances(ctx)
	if err != nil {
		b.log.Warn("balances unreadable", "err", err)

		return false
	}

	base, quote := strategy.Spend(rfq.TakerBuys, rfq.Qty, price, rfq.FeeBps)
	share := new(big.Rat).SetFloat64(b.config.MaxSpendShare)

	if exceeds(base, balances[rfq.Base], share) || exceeds(quote, balances[rfq.Quote], share) {
		b.skip(rfq, "the trade would take more than the allowed share of a free balance: "+
			strategy.Format(base)+" "+rfq.Base+" and "+strategy.Format(quote)+" "+rfq.Quote)

		return false
	}

	return true
}

func exceeds(spend, available, share *big.Rat) bool {
	if spend.Sign() == 0 {
		return false
	}

	if available == nil {
		return true
	}

	return spend.Cmp(new(big.Rat).Mul(available, share)) > 0
}

func (b *Bot) skip(rfq desk.RFQ, reason string) {
	b.log.Info("rfq skipped", "rfq", rfq.ID, "base", rfq.Base, "quote_asset", rfq.Quote, "reason", reason)
}
