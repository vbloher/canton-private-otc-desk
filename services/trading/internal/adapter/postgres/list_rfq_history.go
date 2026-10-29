package postgres

import (
	"context"
	"fmt"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/trading/internal/domain"
)

func (p *Postgres) ListRFQHistory(ctx context.Context, taker string, limit int) ([]domain.RFQ, error) {
	ctx, span := tracer.Start(ctx, "adapter postgres ListRFQHistory")
	defer span.End()

	const sql = `SELECT ` + rfqColumns + `
                 FROM rfqs
                 WHERE taker_party = $1 AND status IN ('expired', 'cancelled')
                 ORDER BY closed_at DESC
                 LIMIT $2`

	txOrPool := p.tx.TryExtractTX(ctx)

	rows, err := txOrPool.Query(ctx, sql, taker, limit)
	if err != nil {
		return nil, fmt.Errorf("txOrPool.Query: %w", err)
	}

	rfqs, err := collectRFQs(rows)
	if err != nil {
		return nil, fmt.Errorf("collectRFQs: %w", err)
	}

	return rfqs, nil
}
