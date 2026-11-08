package usecase

import (
	"context"
	"fmt"

	"github.com/vbloher/canton-private-otc-desk/pkg/outbox"
	"github.com/vbloher/canton-private-otc-desk/pkg/tradeevents"
	"github.com/vbloher/canton-private-otc-desk/services/trading/internal/domain"
)

func (u *UseCase) announceSettled(ctx context.Context, t domain.Trade) error {
	taker, maker := t.Exchanges()

	m, err := outbox.NewMessage(tradeevents.Topic, t.ID.String(), tradeevents.TypeSettled, tradeevents.Settled{
		TradeID:   t.ID,
		UpdateID:  t.UpdateID,
		SettledAt: *t.SettledAt,
		Taker:     side(taker),
		Maker:     side(maker),
	})
	if err != nil {
		return fmt.Errorf("outbox.NewMessage: %w", err)
	}

	err = u.outbox.Add(ctx, m)
	if err != nil {
		return fmt.Errorf("outbox.Add: %w", err)
	}

	return nil
}

func side(e domain.Exchange) tradeevents.Side {
	return tradeevents.Side{PartyID: e.Party, Gave: amount(e.Gave), Received: amount(e.Received)}
}

func amount(a domain.Amount) tradeevents.Amount {
	return tradeevents.Amount{Symbol: a.Instrument, Value: a.Value}
}
