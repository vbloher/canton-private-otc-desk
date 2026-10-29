package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/trading/internal/domain"
)

const uniqueViolation = "23505"

func (p *Postgres) CreateQuote(ctx context.Context, q domain.Quote) error {
	ctx, span := tracer.Start(ctx, "adapter postgres CreateQuote")
	defer span.End()

	const sql = `INSERT INTO quotes (id, rfq_id, maker_party, maker_hint, price, valid_until, status, created_at,
                                     opened_at)
                 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`

	args := []any{
		q.ID,
		q.RFQID,
		q.Maker.ID,
		q.Maker.Hint,
		q.Price.String(),
		q.ValidUntil,
		string(q.Status),
		q.CreatedAt,
		q.OpenedAt,
	}

	txOrPool := p.tx.TryExtractTX(ctx)

	_, err := txOrPool.Exec(ctx, sql, args...)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
			return domain.ErrAlreadyQuoted
		}

		return fmt.Errorf("txOrPool.Exec: %w", err)
	}

	return nil
}
