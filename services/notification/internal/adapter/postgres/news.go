package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
)

func (p *Postgres) NewsWanted(ctx context.Context, email string) (bool, error) {
	ctx, span := tracer.Start(ctx, "adapter postgres NewsWanted")
	defer span.End()

	const sql = `SELECT EXISTS (SELECT 1 FROM profiles WHERE email = $1 AND confirmed_at IS NOT NULL AND news)`

	var wanted bool

	err := p.tx.TryExtractTX(ctx).QueryRow(ctx, sql, email).Scan(&wanted)
	if err != nil {
		return false, fmt.Errorf("row.Scan: %w", err)
	}

	return wanted, nil
}

func (p *Postgres) ClearNews(ctx context.Context, email string) ([]string, error) {
	ctx, span := tracer.Start(ctx, "adapter postgres ClearNews")
	defer span.End()

	const sql = `UPDATE profiles SET news = FALSE, updated_at = now() WHERE email = $1 AND news RETURNING party_id`

	rows, err := p.tx.TryExtractTX(ctx).Query(ctx, sql, email)
	if err != nil {
		return nil, fmt.Errorf("txOrPool.Query: %w", err)
	}

	parties, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, fmt.Errorf("pgx.CollectRows: %w", err)
	}

	return parties, nil
}
