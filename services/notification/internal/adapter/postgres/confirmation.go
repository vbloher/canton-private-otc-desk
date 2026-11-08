package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/notification/internal/domain"
)

func (p *Postgres) ReplaceConfirmation(ctx context.Context, c domain.Confirmation) error {
	ctx, span := tracer.Start(ctx, "adapter postgres ReplaceConfirmation")
	defer span.End()

	txOrPool := p.tx.TryExtractTX(ctx)

	_, err := txOrPool.Exec(ctx, `DELETE FROM email_confirmations WHERE party_id = $1`, c.PartyID)
	if err != nil {
		return fmt.Errorf("delete: %w", err)
	}

	const sql = `INSERT INTO email_confirmations (token_hash, party_id, email, expires_at) VALUES ($1, $2, $3, $4)`

	_, err = txOrPool.Exec(ctx, sql, c.TokenHash, c.PartyID, c.Email, c.ExpiresAt)
	if err != nil {
		return fmt.Errorf("insert: %w", err)
	}

	return nil
}

func (p *Postgres) GetConfirmation(ctx context.Context, tokenHash string) (domain.Confirmation, error) {
	ctx, span := tracer.Start(ctx, "adapter postgres GetConfirmation")
	defer span.End()

	const sql = `SELECT token_hash, party_id, email, expires_at, confirmed_at
                 FROM email_confirmations
                 WHERE token_hash = $1`

	var c domain.Confirmation

	err := p.tx.TryExtractTX(ctx).QueryRow(ctx, sql, tokenHash).
		Scan(&c.TokenHash, &c.PartyID, &c.Email, &c.ExpiresAt, &c.ConfirmedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Confirmation{}, domain.ErrNotFound
		}

		return domain.Confirmation{}, fmt.Errorf("row.Scan: %w", err)
	}

	return c, nil
}

func (p *Postgres) MarkConfirmed(ctx context.Context, tokenHash string, at time.Time) error {
	ctx, span := tracer.Start(ctx, "adapter postgres MarkConfirmed")
	defer span.End()

	_, err := p.tx.TryExtractTX(ctx).Exec(ctx,
		`UPDATE email_confirmations SET confirmed_at = $2 WHERE token_hash = $1`, tokenHash, at)
	if err != nil {
		return fmt.Errorf("txOrPool.Exec: %w", err)
	}

	return nil
}
