package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
)

func (p *Postgres) Cleanup(ctx context.Context, now, finishedBefore time.Time) error {
	ctx, span := tracer.Start(ctx, "adapter postgres Cleanup")
	defer span.End()

	txOrPool := p.tx.TryExtractTX(ctx)

	_, err := txOrPool.Exec(ctx, `DELETE FROM email_confirmations WHERE expires_at < $1`, now)
	if err != nil {
		return fmt.Errorf("delete confirmations: %w", err)
	}

	_, err = txOrPool.Exec(ctx, `DELETE FROM emails WHERE status <> 'pending' AND finished_at < $1`, finishedBefore)
	if err != nil {
		return fmt.Errorf("delete emails: %w", err)
	}

	return nil
}
