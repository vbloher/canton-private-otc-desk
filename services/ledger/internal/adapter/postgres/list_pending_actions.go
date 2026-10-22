package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/domain"
)

// ListPendingActions are the party's actions still open for a signature, oldest first.
func (p *Postgres) ListPendingActions(ctx context.Context, partyID string, now time.Time) ([]domain.Action, error) {
	ctx, span := tracer.Start(ctx, "adapter postgres ListPendingActions")
	defer span.End()

	const sql = `SELECT ` + actionColumns + ` FROM actions
                    WHERE party_id = $1 AND status = $2 AND expires_at > $3
                    ORDER BY created_at, id`

	txOrPool := p.tx.TryExtractTX(ctx)

	rows, err := txOrPool.Query(ctx, sql, partyID, string(domain.ActionPending), now)
	if err != nil {
		return nil, fmt.Errorf("txOrPool.Query: %w", err)
	}

	actions, err := scanActions(rows)
	if err != nil {
		return nil, fmt.Errorf("scanActions: %w", err)
	}

	return actions, nil
}
