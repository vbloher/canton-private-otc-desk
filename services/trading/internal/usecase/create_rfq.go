package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/vbloher/canton-private-otc-desk/pkg/decimal"
	"github.com/vbloher/canton-private-otc-desk/pkg/events"
	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/trading/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/trading/internal/dto"
)

func (u *UseCase) CreateRFQ(ctx context.Context, input dto.CreateRFQInput) (domain.RFQ, error) {
	ctx, span := tracer.Start(ctx, "usecase CreateRFQ")
	defer span.End()

	direction, err := domain.ParseDirection(input.Direction)
	if err != nil {
		return domain.RFQ{}, fmt.Errorf("domain.ParseDirection: %w", err)
	}

	qty, err := decimal.Parse(input.Qty)
	if err != nil {
		return domain.RFQ{}, domain.Errorf(domain.ErrValidation, "qty: %w", err)
	}

	desk, err := u.desk(ctx)
	if err != nil {
		return domain.RFQ{}, fmt.Errorf("u.desk: %w", err)
	}

	makers, err := u.resolveMakers(ctx, input.Makers)
	if err != nil {
		return domain.RFQ{}, fmt.Errorf("u.resolveMakers: %w", err)
	}

	rfq, err := domain.NewRFQ(domain.RFQParams{
		Taker:     input.Taker,
		Direction: direction,
		Qty:       qty,
		Base:      input.Base,
		Quote:     input.Quote,
		Makers:    makers,
		TTL:       time.Duration(input.TTLSeconds) * time.Second,
		FeeBps:    u.config.FeeBps,
	}, desk, time.Now())
	if err != nil {
		return domain.RFQ{}, fmt.Errorf("domain.NewRFQ: %w", err)
	}

	err = u.postgres.CreateRFQ(ctx, rfq)
	if err != nil {
		return domain.RFQ{}, fmt.Errorf("postgres.CreateRFQ: %w", err)
	}

	u.publishRFQ(ctx, events.RFQCreated, rfq)

	return rfq, nil
}

func (u *UseCase) resolveMakers(ctx context.Context, hints []string) ([]domain.Party, error) {
	found, err := u.identity.ResolveMakers(ctx, hints)
	if err != nil {
		return nil, fmt.Errorf("identity.ResolveMakers: %w", err)
	}

	byHint := make(map[string]domain.Party, len(found))
	for _, p := range found {
		byHint[p.Hint] = p
	}

	makers := make([]domain.Party, 0, len(hints))

	for _, hint := range hints {
		p, ok := byHint[hint]
		if !ok {
			return nil, domain.Errorf(domain.ErrNotAMaker, "%q", hint)
		}

		makers = append(makers, p)
	}

	return makers, nil
}
