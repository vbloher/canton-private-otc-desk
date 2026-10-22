package postgres

import (
	"context"
	"fmt"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/trading/internal/domain"
)

func (p *Postgres) CreateTrade(ctx context.Context, t domain.Trade) error {
	ctx, span := tracer.Start(ctx, "adapter postgres CreateTrade")
	defer span.End()

	const sql = `INSERT INTO trades (id, rfq_id, quote_id, taker_party, taker_hint, maker_party, maker_hint,
                                     direction, base, quote, qty, price, notional, fee, status, reason,
                                     update_id, created_at, settled_at)
                 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19)`

	args := []any{
		t.ID,
		t.RFQID,
		t.QuoteID,
		t.Taker.ID,
		t.Taker.Hint,
		t.Maker.ID,
		t.Maker.Hint,
		string(t.Direction),
		t.Base,
		t.Quote,
		t.Qty.String(),
		t.Price.String(),
		t.Notional.String(),
		t.Fee.String(),
		string(t.Status),
		t.Reason,
		t.UpdateID,
		t.CreatedAt,
		t.SettledAt,
	}

	txOrPool := p.tx.TryExtractTX(ctx)

	_, err := txOrPool.Exec(ctx, sql, args...)
	if err != nil {
		return fmt.Errorf("txOrPool.Exec: %w", err)
	}

	return nil
}
