package postgres

import (
	"context"

	"github.com/google/uuid"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/trading/internal/domain"
)

func (p *Postgres) GetQuote(ctx context.Context, id uuid.UUID) (domain.Quote, error) {
	ctx, span := tracer.Start(ctx, "adapter postgres GetQuote")
	defer span.End()

	return p.getQuote(ctx, getQuoteSQL, id)
}
