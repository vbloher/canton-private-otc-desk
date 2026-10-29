package postgres

import (
	"context"
	"fmt"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/trading/internal/domain"
)

func (p *Postgres) UpdateRFQ(ctx context.Context, rfq domain.RFQ) error {
	ctx, span := tracer.Start(ctx, "adapter postgres UpdateRFQ")
	defer span.End()

	const sql = `UPDATE rfqs SET status = $2, closed_at = $3 WHERE id = $1`

	txOrPool := p.tx.TryExtractTX(ctx)

	tag, err := txOrPool.Exec(ctx, sql, rfq.ID, string(rfq.Status), rfq.ClosedAt)
	if err != nil {
		return fmt.Errorf("txOrPool.Exec: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}

	return nil
}
