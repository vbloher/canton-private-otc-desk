package usecase

import (
	"context"
	"errors"
	"fmt"

	"github.com/rs/zerolog/log"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/maker-bot/internal/domain"
)

// Tick quotes, for every configured maker, each open RFQ the maker has not quoted yet.
func (u *UseCase) Tick(ctx context.Context) error {
	ctx, span := tracer.Start(ctx, "usecase Tick")
	defer span.End()

	errs := make([]error, 0, len(u.config.Makers))

	for _, maker := range u.config.Makers {
		errs = append(errs, u.quote(ctx, maker))
	}

	return errors.Join(errs...)
}

func (u *UseCase) quote(ctx context.Context, maker domain.Maker) error {
	rfqs, err := u.gateway.ListIncomingRFQs(ctx, maker.APIKey)
	if err != nil {
		return fmt.Errorf("gateway.ListIncomingRFQs: %w", err)
	}

	var errs []error

	for _, rfq := range rfqs {
		if !rfq.Open || rfq.Quoted {
			continue
		}

		price, err := domain.Price(rfq.Direction, rfq.Base, rfq.Quote, u.config.Prices, maker.SpreadBps)
		if err != nil {
			errs = append(errs, fmt.Errorf("rfq %s: domain.Price: %w", rfq.ID, err))

			continue
		}

		err = u.gateway.SubmitQuote(ctx, maker.APIKey, rfq.ID, price, u.config.QuoteTTL)
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
