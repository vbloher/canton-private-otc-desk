package usecase

import (
	"context"

	"github.com/rs/zerolog/log"

	"github.com/vbloher/canton-private-otc-desk/pkg/events"
	"github.com/vbloher/canton-private-otc-desk/services/trading/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/trading/internal/dto"
)

// publish never fails the caller: the bus is at-most-once and clients re-fetch over REST. It runs
// after a commit, so a caller that has gone away does not stop it.
func (u *UseCase) publish(ctx context.Context, ev events.Event, partyIDs ...string) {
	err := u.events.Publish(context.WithoutCancel(ctx), ev, partyIDs...)
	if err != nil {
		log.Error().Err(err).Str("event", ev.Type).Msg("usecase: events.Publish")
	}
}

// publishRFQ tells the taker and every maker of the RFQ.
func (u *UseCase) publishRFQ(ctx context.Context, eventType string, rfq domain.RFQ) {
	u.publish(ctx, events.Event{Type: eventType, Data: dto.RFQEvent{RFQID: rfq.ID}}, rfq.Parties()...)
}

// publishQuote tells the quote's maker, and the taker only of a quote the taker may see.
func (u *UseCase) publishQuote(ctx context.Context, taker string, q domain.Quote) {
	ev := events.Event{
		Type: events.QuoteUpdated,
		Data: dto.QuoteEvent{QuoteID: q.ID, RFQID: q.RFQID, Status: q.Status},
	}

	if !q.VisibleToTaker() {
		u.publish(ctx, ev, q.Maker.ID)

		return
	}

	u.publish(ctx, ev, taker, q.Maker.ID)
}

// publishTrade tells both sides of the trade.
func (u *UseCase) publishTrade(ctx context.Context, t domain.Trade) {
	ev := events.Event{
		Type: events.TradeUpdated,
		Data: dto.TradeEvent{TradeID: t.ID, RFQID: t.RFQID, Status: t.Status},
	}

	u.publish(ctx, ev, t.Taker.ID, t.Maker.ID)
}
