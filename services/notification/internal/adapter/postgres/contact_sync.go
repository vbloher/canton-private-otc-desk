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

func (p *Postgres) QueueContactSync(ctx context.Context, email string, now time.Time) error {
	ctx, span := tracer.Start(ctx, "adapter postgres QueueContactSync")
	defer span.End()

	const sql = `INSERT INTO contact_syncs (email, next_attempt_at)
                 VALUES ($1, $2)
                 ON CONFLICT (email) DO UPDATE SET attempts = 0, next_attempt_at = EXCLUDED.next_attempt_at`

	_, err := p.tx.TryExtractTX(ctx).Exec(ctx, sql, email, now)
	if err != nil {
		return fmt.Errorf("txOrPool.Exec: %w", err)
	}

	return nil
}

func (p *Postgres) NextDueContactSync(ctx context.Context, now time.Time) (domain.ContactSync, error) {
	ctx, span := tracer.Start(ctx, "adapter postgres NextDueContactSync")
	defer span.End()

	const sql = `SELECT email, attempts, next_attempt_at
                 FROM contact_syncs
                 WHERE next_attempt_at <= $1
                 ORDER BY next_attempt_at
                 LIMIT 1
                 FOR UPDATE SKIP LOCKED`

	var s domain.ContactSync

	err := p.tx.TryExtractTX(ctx).QueryRow(ctx, sql, now).Scan(&s.Email, &s.Attempts, &s.NextAttemptAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ContactSync{}, domain.ErrNotFound
		}

		return domain.ContactSync{}, fmt.Errorf("row.Scan: %w", err)
	}

	return s, nil
}

func (p *Postgres) RescheduleContactSync(ctx context.Context, s domain.ContactSync) error {
	ctx, span := tracer.Start(ctx, "adapter postgres RescheduleContactSync")
	defer span.End()

	_, err := p.tx.TryExtractTX(ctx).Exec(ctx,
		`UPDATE contact_syncs SET attempts = $2, next_attempt_at = $3 WHERE email = $1`,
		s.Email, s.Attempts, s.NextAttemptAt)
	if err != nil {
		return fmt.Errorf("txOrPool.Exec: %w", err)
	}

	return nil
}

func (p *Postgres) DeleteContactSync(ctx context.Context, email string) error {
	ctx, span := tracer.Start(ctx, "adapter postgres DeleteContactSync")
	defer span.End()

	_, err := p.tx.TryExtractTX(ctx).Exec(ctx, `DELETE FROM contact_syncs WHERE email = $1`, email)
	if err != nil {
		return fmt.Errorf("txOrPool.Exec: %w", err)
	}

	return nil
}
