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

func (p *Postgres) CreateEmail(ctx context.Context, e domain.Email) error {
	ctx, span := tracer.Start(ctx, "adapter postgres CreateEmail")
	defer span.End()

	const sql = `INSERT INTO emails (id, dedupe_key, recipient, kind, subject, html, text, status, attempts,
                                     next_attempt_at, created_at)
                 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
                 ON CONFLICT (dedupe_key) DO NOTHING`

	_, err := p.tx.TryExtractTX(ctx).Exec(ctx, sql, e.ID, e.DedupeKey, e.Recipient, string(e.Kind), e.Subject, e.HTML,
		e.Text, string(e.Status), e.Attempts, e.NextAttemptAt, e.CreatedAt)
	if err != nil {
		return fmt.Errorf("txOrPool.Exec: %w", err)
	}

	return nil
}

func (p *Postgres) NextDueEmail(ctx context.Context, now time.Time) (domain.Email, error) {
	ctx, span := tracer.Start(ctx, "adapter postgres NextDueEmail")
	defer span.End()

	const sql = `SELECT id, dedupe_key, recipient, kind, subject, html, text, status, attempts, next_attempt_at,
                        provider_id, last_error, created_at, finished_at
                 FROM emails
                 WHERE status = 'pending' AND next_attempt_at <= $1
                 ORDER BY next_attempt_at
                 LIMIT 1
                 FOR UPDATE SKIP LOCKED`

	var (
		e      domain.Email
		kind   string
		status string
	)

	err := p.tx.TryExtractTX(ctx).QueryRow(ctx, sql, now).Scan(&e.ID, &e.DedupeKey, &e.Recipient, &kind, &e.Subject,
		&e.HTML, &e.Text, &status, &e.Attempts, &e.NextAttemptAt, &e.ProviderID, &e.LastError, &e.CreatedAt, &e.FinishedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Email{}, domain.ErrNotFound
		}

		return domain.Email{}, fmt.Errorf("row.Scan: %w", err)
	}

	e.Kind = domain.EmailKind(kind)
	e.Status = domain.DeliveryStatus(status)

	return e, nil
}

func (p *Postgres) UpdateEmail(ctx context.Context, e domain.Email) error {
	ctx, span := tracer.Start(ctx, "adapter postgres UpdateEmail")
	defer span.End()

	const sql = `UPDATE emails
                 SET recipient = $2, subject = $3, html = $4, text = $5, status = $6, attempts = $7,
                     next_attempt_at = $8, provider_id = $9, last_error = $10, finished_at = $11
                 WHERE id = $1`

	tag, err := p.tx.TryExtractTX(ctx).Exec(ctx, sql, e.ID, e.Recipient, e.Subject, e.HTML, e.Text, string(e.Status),
		e.Attempts, e.NextAttemptAt, e.ProviderID, e.LastError, e.FinishedAt)
	if err != nil {
		return fmt.Errorf("txOrPool.Exec: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}

	return nil
}
