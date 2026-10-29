package postgres

import (
	"context"
	"fmt"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/domain"
)

func (p *Postgres) UpdateAction(ctx context.Context, action domain.Action) error {
	ctx, span := tracer.Start(ctx, "adapter postgres UpdateAction")
	defer span.End()

	const sql = `UPDATE actions SET status = $1, error = $2 WHERE id = $3`

	txOrPool := p.tx.TryExtractTX(ctx)

	tag, err := txOrPool.Exec(ctx, sql, string(action.Status), action.Error, action.ID)
	if err != nil {
		return fmt.Errorf("txOrPool.Exec: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return domain.Errorf(domain.ErrNotFound, "action %s", action.ID)
	}

	return nil
}
