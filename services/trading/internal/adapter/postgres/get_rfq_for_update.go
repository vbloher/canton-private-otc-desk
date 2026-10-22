package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/trading/internal/domain"
)

func (p *Postgres) GetRFQForUpdate(ctx context.Context, id uuid.UUID) (domain.RFQ, error) {
	ctx, span := tracer.Start(ctx, "adapter postgres GetRFQForUpdate")
	defer span.End()

	const sql = `SELECT ` + rfqColumns + ` FROM rfqs WHERE id = $1 FOR UPDATE`

	var r rfqRow

	txOrPool := p.tx.TryExtractTX(ctx)

	err := txOrPool.QueryRow(ctx, sql, id).Scan(r.dest()...)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.RFQ{}, domain.ErrNotFound
		}

		return domain.RFQ{}, fmt.Errorf("txOrPool.QueryRow: %w", err)
	}

	rfq, err := r.toDomain()
	if err != nil {
		return domain.RFQ{}, fmt.Errorf("r.toDomain: %w", err)
	}

	return rfq, nil
}
