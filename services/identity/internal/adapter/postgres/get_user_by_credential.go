package postgres

import (
	"context"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/domain"
)

func (p *Postgres) GetUserByCredential(ctx context.Context, credentialID string) (domain.User, error) {
	ctx, span := tracer.Start(ctx, "adapter postgres GetUserByCredential")
	defer span.End()

	const sql = `SELECT ` + userColumns + ` FROM users WHERE credential_id = $1`

	return p.getUser(ctx, sql, credentialID)
}
