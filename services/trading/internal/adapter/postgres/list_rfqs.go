package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/trading/internal/domain"
)

func (p *Postgres) ListRFQs(ctx context.Context, ids []uuid.UUID) ([]domain.RFQ, error) {
	ctx, span := tracer.Start(ctx, "adapter postgres ListRFQs")
	defer span.End()

	const sql = `SELECT ` + rfqColumns + ` FROM rfqs WHERE id = ANY($1)`

	txOrPool := p.tx.TryExtractTX(ctx)

	rows, err := txOrPool.Query(ctx, sql, ids)
	if err != nil {
		return nil, fmt.Errorf("txOrPool.Query: %w", err)
	}

	rfqs, err := collectRFQs(rows)
	if err != nil {
		return nil, fmt.Errorf("collectRFQs: %w", err)
	}

	return rfqs, nil
}
