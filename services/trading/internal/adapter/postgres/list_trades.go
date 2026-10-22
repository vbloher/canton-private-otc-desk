package postgres

import (
	"context"
	"fmt"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/trading/internal/domain"
)

// ListTrades returns the party's trades in either role, newest first.
func (p *Postgres) ListTrades(ctx context.Context, party string, limit int) ([]domain.Trade, error) {
	ctx, span := tracer.Start(ctx, "adapter postgres ListTrades")
	defer span.End()

	const sql = `SELECT ` + tradeColumns + `
                 FROM trades
                 WHERE taker_party = $1 OR maker_party = $1
                 ORDER BY created_at DESC
                 LIMIT $2`

	txOrPool := p.tx.TryExtractTX(ctx)

	rows, err := txOrPool.Query(ctx, sql, party, limit)
	if err != nil {
		return nil, fmt.Errorf("txOrPool.Query: %w", err)
	}

	trades, err := collectTrades(rows)
	if err != nil {
		return nil, fmt.Errorf("collectTrades: %w", err)
	}

	return trades, nil
}
