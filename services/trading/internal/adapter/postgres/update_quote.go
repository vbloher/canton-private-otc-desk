package postgres

import (
	"context"
	"fmt"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/trading/internal/domain"
)

func (p *Postgres) UpdateQuote(ctx context.Context, q domain.Quote) error {
	ctx, span := tracer.Start(ctx, "adapter postgres UpdateQuote")
	defer span.End()

	const sql = `UPDATE quotes SET status = $2, opened_at = $3 WHERE id = $1`

	txOrPool := p.tx.TryExtractTX(ctx)

	tag, err := txOrPool.Exec(ctx, sql, q.ID, string(q.Status), q.OpenedAt)
	if err != nil {
		return fmt.Errorf("txOrPool.Exec: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}

	return nil
}
