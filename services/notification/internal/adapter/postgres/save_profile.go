package postgres

import (
	"context"
	"fmt"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/notification/internal/domain"
)

func (p *Postgres) SaveProfile(ctx context.Context, profile domain.Profile) error {
	ctx, span := tracer.Start(ctx, "adapter postgres SaveProfile")
	defer span.End()

	const sql = `INSERT INTO profiles (party_id, email, confirmed_at, news, trades)
                 VALUES ($1, $2, $3, $4, $5)
                 ON CONFLICT (party_id) DO UPDATE
                 SET email        = EXCLUDED.email,
                     confirmed_at = EXCLUDED.confirmed_at,
                     news         = EXCLUDED.news,
                     trades       = EXCLUDED.trades,
                     updated_at   = now()`

	_, err := p.tx.TryExtractTX(ctx).Exec(ctx, sql,
		profile.PartyID, nullText(profile.Email), profile.ConfirmedAt, profile.News, profile.Trades)
	if err != nil {
		return fmt.Errorf("txOrPool.Exec: %w", err)
	}

	return nil
}
