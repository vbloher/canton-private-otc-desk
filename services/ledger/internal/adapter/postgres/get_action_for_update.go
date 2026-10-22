package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/domain"
)

func (p *Postgres) GetActionForUpdate(ctx context.Context, id uuid.UUID) (domain.Action, error) {
	ctx, span := tracer.Start(ctx, "adapter postgres GetActionForUpdate")
	defer span.End()

	const sql = `SELECT ` + actionColumns + ` FROM actions WHERE id = $1 FOR UPDATE`

	txOrPool := p.tx.TryExtractTX(ctx)

	action, err := scanAction(txOrPool.QueryRow(ctx, sql, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Action{}, domain.Errorf(domain.ErrNotFound, "action %s", id)
		}

		return domain.Action{}, fmt.Errorf("txOrPool.QueryRow: %w", err)
	}

	return action, nil
}
