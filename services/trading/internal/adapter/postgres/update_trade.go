package postgres

import (
	"context"
	"fmt"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/trading/internal/domain"
)

// UpdateTrade saves the outcome of a trade: status, failure reason, settling update and time.
func (p *Postgres) UpdateTrade(ctx context.Context, t domain.Trade) error {
	ctx, span := tracer.Start(ctx, "adapter postgres UpdateTrade")
	defer span.End()

	const sql = `UPDATE trades SET status = $2, reason = $3, update_id = $4, settled_at = $5 WHERE id = $1`

	txOrPool := p.tx.TryExtractTX(ctx)

	tag, err := txOrPool.Exec(ctx, sql, t.ID, string(t.Status), t.Reason, t.UpdateID, t.SettledAt)
	if err != nil {
		return fmt.Errorf("txOrPool.Exec: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}

	return nil
}
