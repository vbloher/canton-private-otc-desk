package postgres

import (
	"context"
	"fmt"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/domain"
)

// ExpireActionsByRef expires every pending action of what ref names (a settlement) and
// returns them. A row an execution holds is updated once that execution has finished, if
// it is still pending then.
func (p *Postgres) ExpireActionsByRef(ctx context.Context, ref string) ([]domain.Action, error) {
	ctx, span := tracer.Start(ctx, "adapter postgres ExpireActionsByRef")
	defer span.End()

	const sql = `UPDATE actions SET status = $1
                    WHERE ref = $2 AND status = $3
                    RETURNING ` + actionColumns

	txOrPool := p.tx.TryExtractTX(ctx)

	rows, err := txOrPool.Query(ctx, sql, string(domain.ActionExpired), ref, string(domain.ActionPending))
	if err != nil {
		return nil, fmt.Errorf("txOrPool.Query: %w", err)
	}

	actions, err := scanActions(rows)
	if err != nil {
		return nil, fmt.Errorf("scanActions: %w", err)
	}

	return actions, nil
}
