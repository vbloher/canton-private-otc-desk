package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/trading/internal/domain"
)

// ListMakerRFQs returns the open RFQs sent to the maker and those closed since, newest first.
func (p *Postgres) ListMakerRFQs(ctx context.Context, maker string, since time.Time) ([]domain.RFQ, error) {
	ctx, span := tracer.Start(ctx, "adapter postgres ListMakerRFQs")
	defer span.End()

	const sql = `SELECT ` + rfqColumns + `
                 FROM rfqs
                 WHERE makers @> jsonb_build_array(jsonb_build_object('id', $1::text))
                   AND (status = 'open' OR closed_at >= $2)
                 ORDER BY created_at DESC`

	txOrPool := p.tx.TryExtractTX(ctx)

	rows, err := txOrPool.Query(ctx, sql, maker, since)
	if err != nil {
		return nil, fmt.Errorf("txOrPool.Query: %w", err)
	}

	rfqs, err := collectRFQs(rows)
	if err != nil {
		return nil, fmt.Errorf("collectRFQs: %w", err)
	}

	return rfqs, nil
}
