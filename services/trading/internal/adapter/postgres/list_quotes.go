package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/trading/internal/domain"
)

func (p *Postgres) ListQuotes(ctx context.Context, rfqIDs []uuid.UUID) ([]domain.Quote, error) {
	ctx, span := tracer.Start(ctx, "adapter postgres ListQuotes")
	defer span.End()

	const sql = `SELECT ` + quoteColumns + ` FROM quotes WHERE rfq_id = ANY($1) ORDER BY created_at, id`

	txOrPool := p.tx.TryExtractTX(ctx)

	rows, err := txOrPool.Query(ctx, sql, rfqIDs)
	if err != nil {
		return nil, fmt.Errorf("txOrPool.Query: %w", err)
	}

	quotes, err := collectQuotes(rows)
	if err != nil {
		return nil, fmt.Errorf("collectQuotes: %w", err)
	}

	return quotes, nil
}
