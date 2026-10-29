package postgres

import (
	"context"
	"fmt"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/trading/internal/domain"
)

func (p *Postgres) ListQuotesByStatus(ctx context.Context, statuses ...domain.QuoteStatus) ([]domain.Quote, error) {
	ctx, span := tracer.Start(ctx, "adapter postgres ListQuotesByStatus")
	defer span.End()

	const sql = `SELECT ` + quoteColumns + ` FROM quotes WHERE status = ANY($1) ORDER BY created_at, id`

	names := make([]string, 0, len(statuses))
	for _, s := range statuses {
		names = append(names, string(s))
	}

	txOrPool := p.tx.TryExtractTX(ctx)

	rows, err := txOrPool.Query(ctx, sql, names)
	if err != nil {
		return nil, fmt.Errorf("txOrPool.Query: %w", err)
	}

	quotes, err := collectQuotes(rows)
	if err != nil {
		return nil, fmt.Errorf("collectQuotes: %w", err)
	}

	return quotes, nil
}
