package postgres

import (
	"context"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/domain"
)

func (p *Postgres) GetUserByParty(ctx context.Context, partyID string) (domain.User, error) {
	ctx, span := tracer.Start(ctx, "adapter postgres GetUserByParty")
	defer span.End()

	const sql = `SELECT ` + userColumns + ` FROM users WHERE party_id = $1`

	return p.getUser(ctx, sql, partyID)
}
