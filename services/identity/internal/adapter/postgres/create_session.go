package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
)

func (p *Postgres) CreateSession(ctx context.Context, tokenHash string, userID uuid.UUID, expiresAt time.Time) error {
	ctx, span := tracer.Start(ctx, "adapter postgres CreateSession")
	defer span.End()

	const sql = `INSERT INTO sessions (token_hash, user_id, expires_at) VALUES ($1, $2, $3)`

	txOrPool := p.tx.TryExtractTX(ctx)

	_, err := txOrPool.Exec(ctx, sql, tokenHash, userID, expiresAt)
	if err != nil {
		return fmt.Errorf("txOrPool.Exec: %w", err)
	}

	return nil
}
