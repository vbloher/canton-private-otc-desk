package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/pkg/tradeevents"
	"github.com/vbloher/canton-private-otc-desk/services/notification/internal/domain"
)

func (u *UseCase) NotifyTradeSettled(ctx context.Context, event tradeevents.Settled) error {
	ctx, span := tracer.Start(ctx, "usecase NotifyTradeSettled")
	defer span.End()

	if !u.config.Enabled {
		return nil
	}

	profiles, err := u.postgres.ListProfiles(ctx, []string{event.Taker.PartyID, event.Maker.PartyID})
	if err != nil {
		return fmt.Errorf("postgres.ListProfiles: %w", err)
	}

	byParty := make(map[string]domain.Profile, len(profiles))
	for _, profile := range profiles {
		byParty[profile.PartyID] = profile
	}

	now := time.Now()

	err = u.tx.Wrap(ctx, func(ctx context.Context) error {
		for _, side := range []tradeevents.Side{event.Taker, event.Maker} {
			profile, found := byParty[side.PartyID]
			if !found || !profile.WantsTrades() {
				continue
			}

			err := u.queueTradeLetter(ctx, event, side, profile.Email, now)
			if err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("transaction.Wrap: %w", err)
	}

	return nil
}

func (u *UseCase) queueTradeLetter(
	ctx context.Context, event tradeevents.Settled, side tradeevents.Side, email string, now time.Time,
) error {
	facts := domain.TradeFacts{
		Gave:     domain.FormatAmount(side.Gave.Value, side.Gave.Symbol),
		Received: domain.FormatAmount(side.Received.Value, side.Received.Symbol),
		BaseURL:  u.config.baseURL(),
	}

	if u.config.IncludeUpdateID {
		facts.UpdateID = event.UpdateID
	}

	letter, err := domain.TradeLetter(facts)
	if err != nil {
		return fmt.Errorf("domain.TradeLetter: %w", err)
	}

	key := "trade-settled:" + event.TradeID.String() + ":" + side.PartyID

	err = u.postgres.CreateEmail(ctx, domain.NewEmail(key, email, domain.KindTradeSettled, letter, now))
	if err != nil {
		return fmt.Errorf("postgres.CreateEmail: %w", err)
	}

	return nil
}
