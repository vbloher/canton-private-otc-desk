package postgres

import (
	"context"
	"fmt"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
)

func (p *Postgres) DeleteSession(ctx context.Context, tokenHash string) error {
	ctx, span := tracer.Start(ctx, "adapter postgres DeleteSession")
	defer span.End()

	const sql = `DELETE FROM sessions WHERE token_hash = $1`

	txOrPool := p.tx.TryExtractTX(ctx)

	_, err := txOrPool.Exec(ctx, sql, tokenHash)
	if err != nil {
		return fmt.Errorf("txOrPool.Exec: %w", err)
	}

	return nil
}
