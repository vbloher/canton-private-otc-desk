package postgres

import (
	"context"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/domain"
)

func (p *Postgres) GetUserByAPIKeyHash(ctx context.Context, hash string) (domain.User, error) {
	ctx, span := tracer.Start(ctx, "adapter postgres GetUserByAPIKeyHash")
	defer span.End()

	const sql = `SELECT ` + userColumns + ` FROM users WHERE api_key_hash = $1`

	return p.getUser(ctx, sql, hash)
}
