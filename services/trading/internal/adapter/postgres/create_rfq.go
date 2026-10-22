package postgres

import (
	"context"
	"fmt"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/trading/internal/domain"
)

func (p *Postgres) CreateRFQ(ctx context.Context, rfq domain.RFQ) error {
	ctx, span := tracer.Start(ctx, "adapter postgres CreateRFQ")
	defer span.End()

	const sql = `INSERT INTO rfqs (id, taker_party, taker_hint, direction, qty, base, quote, makers, fee_bps,
                                   deadline, status, created_at, closed_at)
                 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`

	makers, err := marshalMakers(rfq.Makers)
	if err != nil {
		return fmt.Errorf("marshalMakers: %w", err)
	}

	args := []any{
		rfq.ID,
		rfq.TakerParty,
		rfq.TakerHint,
		string(rfq.Direction),
		rfq.Qty.String(),
		rfq.Base,
		rfq.Quote,
		makers,
		rfq.FeeBps,
		rfq.Deadline,
		string(rfq.Status),
		rfq.CreatedAt,
		rfq.ClosedAt,
	}

	txOrPool := p.tx.TryExtractTX(ctx)

	_, err = txOrPool.Exec(ctx, sql, args...)
	if err != nil {
		return fmt.Errorf("txOrPool.Exec: %w", err)
	}

	return nil
}
