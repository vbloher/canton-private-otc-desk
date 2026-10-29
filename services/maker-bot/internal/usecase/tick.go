package usecase

import (
	"context"
	"errors"
	"fmt"

	"github.com/rs/zerolog/log"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/maker-bot/internal/domain"
)

func (u *UseCase) Tick(ctx context.Context) error {
	ctx, span := tracer.Start(ctx, "usecase Tick")
	defer span.End()

	rfqs, err := u.gateway.ListIncomingRFQs(ctx, u.config.APIKey)
	if err != nil {
		return fmt.Errorf("gateway.ListIncomingRFQs: %w", err)
	}

	var errs []error

	for _, rfq := range rfqs {
		if !rfq.Open || rfq.Quoted {
			continue
		}

		price, err := domain.Price(rfq.Direction, rfq.Base, rfq.Quote, u.config.Prices, int64(u.config.Spread))
		if err != nil {
			errs = append(errs, fmt.Errorf("rfq %s: domain.Price: %w", rfq.ID, err))

			continue
		}

		err = u.gateway.SubmitQuote(ctx, u.config.APIKey, rfq.ID, price, u.config.QuoteTTL)
		if errors.Is(err, domain.ErrConflict) {
			log.Info().Err(err).Stringer("rfq", rfq.ID).Msg("usecase Tick: quote refused, skipped")

			continue
		}

		if err != nil {
			errs = append(errs, fmt.Errorf("rfq %s: gateway.SubmitQuote: %w", rfq.ID, err))

			continue
		}

		log.Info().Stringer("rfq", rfq.ID).Stringer("price", price).Msg("usecase Tick: quoted")
	}

	return errors.Join(errs...)
}
