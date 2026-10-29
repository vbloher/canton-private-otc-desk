package postgres

import (
	"context"
	"fmt"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/trading/internal/domain"
)

func (p *Postgres) ListSettlingTrades(ctx context.Context) ([]domain.Trade, error) {
	ctx, span := tracer.Start(ctx, "adapter postgres ListSettlingTrades")
	defer span.End()

	const sql = `SELECT ` + tradeColumns + ` FROM trades WHERE status = 'settling' ORDER BY created_at`

	txOrPool := p.tx.TryExtractTX(ctx)

	rows, err := txOrPool.Query(ctx, sql)
	if err != nil {
		return nil, fmt.Errorf("txOrPool.Query: %w", err)
	}

	trades, err := collectTrades(rows)
	if err != nil {
		return nil, fmt.Errorf("collectTrades: %w", err)
	}

	return trades, nil
}
