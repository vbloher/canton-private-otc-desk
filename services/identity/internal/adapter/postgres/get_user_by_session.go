package postgres

import (
	"context"
	"time"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/domain"
)

// GetUserBySession finds the owner of a session that has not expired at now.
func (p *Postgres) GetUserBySession(ctx context.Context, tokenHash string, now time.Time) (domain.User, error) {
	ctx, span := tracer.Start(ctx, "adapter postgres GetUserBySession")
	defer span.End()

	const sql = `SELECT ` + userColumns + `
                    FROM users
                    WHERE id = (SELECT user_id FROM sessions WHERE token_hash = $1 AND expires_at > $2)`

	return p.getUser(ctx, sql, tokenHash, now)
}
