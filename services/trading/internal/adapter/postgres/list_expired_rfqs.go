package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/trading/internal/domain"
)

func (p *Postgres) ListExpiredRFQs(ctx context.Context, now time.Time) ([]domain.RFQ, error) {
	ctx, span := tracer.Start(ctx, "adapter postgres ListExpiredRFQs")
	defer span.End()

	const sql = `SELECT ` + rfqColumns + `
                 FROM rfqs r
                 WHERE r.status = 'open' AND r.deadline <= $1
                   AND NOT EXISTS (SELECT 1 FROM trades t WHERE t.rfq_id = r.id AND t.status = 'settling')`

	txOrPool := p.tx.TryExtractTX(ctx)

	rows, err := txOrPool.Query(ctx, sql, now)
	if err != nil {
		return nil, fmt.Errorf("txOrPool.Query: %w", err)
	}

	rfqs, err := collectRFQs(rows)
	if err != nil {
		return nil, fmt.Errorf("collectRFQs: %w", err)
	}

	return rfqs, nil
}
