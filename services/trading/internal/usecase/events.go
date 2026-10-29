package usecase

import (
	"context"

	"github.com/rs/zerolog/log"

	"github.com/vbloher/canton-private-otc-desk/pkg/events"
	"github.com/vbloher/canton-private-otc-desk/services/trading/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/trading/internal/dto"
)

func (u *UseCase) publish(ctx context.Context, ev events.Event, partyIDs ...string) {
	err := u.events.Publish(context.WithoutCancel(ctx), ev, partyIDs...)
	if err != nil {
		log.Error().Err(err).Str("event", ev.Type).Msg("usecase: events.Publish")
	}
}

func (u *UseCase) publishRFQ(ctx context.Context, eventType string, rfq domain.RFQ) {
	u.publish(ctx, events.Event{Type: eventType, Data: dto.RFQEvent{RFQID: rfq.ID}}, rfq.Parties()...)
}

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

func (u *UseCase) publishTrade(ctx context.Context, t domain.Trade) {
	ev := events.Event{
		Type: events.TradeUpdated,
		Data: dto.TradeEvent{TradeID: t.ID, RFQID: t.RFQID, Status: t.Status},
	}

	u.publish(ctx, ev, t.Taker.ID, t.Maker.ID)
}
