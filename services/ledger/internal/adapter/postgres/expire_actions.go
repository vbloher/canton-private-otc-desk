package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/domain"
)

// ExpireActions marks every pending action past its expiry as expired and returns them.
func (p *Postgres) ExpireActions(ctx context.Context, now time.Time) ([]domain.Action, error) {
	ctx, span := tracer.Start(ctx, "adapter postgres ExpireActions")
	defer span.End()

	const sql = `UPDATE actions SET status = $1
                    WHERE status = $2 AND expires_at <= $3
                    RETURNING ` + actionColumns

	txOrPool := p.tx.TryExtractTX(ctx)

	rows, err := txOrPool.Query(ctx, sql, string(domain.ActionExpired), string(domain.ActionPending), now)
	if err != nil {
		return nil, fmt.Errorf("txOrPool.Query: %w", err)
	}

	actions, err := scanActions(rows)
	if err != nil {
		return nil, fmt.Errorf("scanActions: %w", err)
	}

	return actions, nil
}
