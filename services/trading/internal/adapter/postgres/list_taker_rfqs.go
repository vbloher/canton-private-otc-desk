package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/trading/internal/domain"
)

func (p *Postgres) ListTakerRFQs(ctx context.Context, taker string, since time.Time) ([]domain.RFQ, error) {
	ctx, span := tracer.Start(ctx, "adapter postgres ListTakerRFQs")
	defer span.End()

	const sql = `SELECT ` + rfqColumns + `
                 FROM rfqs
                 WHERE taker_party = $1 AND (status = 'open' OR closed_at >= $2)
                 ORDER BY created_at DESC`

	txOrPool := p.tx.TryExtractTX(ctx)

	rows, err := txOrPool.Query(ctx, sql, taker, since)
	if err != nil {
		return nil, fmt.Errorf("txOrPool.Query: %w", err)
	}

	rfqs, err := collectRFQs(rows)
	if err != nil {
		return nil, fmt.Errorf("collectRFQs: %w", err)
	}

	return rfqs, nil
}
