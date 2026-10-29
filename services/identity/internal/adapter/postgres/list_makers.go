package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/domain"
)

func (p *Postgres) ListMakers(ctx context.Context) ([]domain.User, error) {
	ctx, span := tracer.Start(ctx, "adapter postgres ListMakers")
	defer span.End()

	const sql = `SELECT ` + userColumns + ` FROM users WHERE maker_mode ORDER BY display_name, hint`

	txOrPool := p.tx.TryExtractTX(ctx)

	rows, err := txOrPool.Query(ctx, sql)
	if err != nil {
		return nil, fmt.Errorf("txOrPool.Query: %w", err)
	}

	makers, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (domain.User, error) {
		return scanUser(row)
	})
	if err != nil {
		return nil, fmt.Errorf("pgx.CollectRows: %w", err)
	}

	return makers, nil
}
