package postgres

import (
	"context"

	"github.com/google/uuid"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/domain"
)

func (p *Postgres) GetUser(ctx context.Context, id uuid.UUID) (domain.User, error) {
	ctx, span := tracer.Start(ctx, "adapter postgres GetUser")
	defer span.End()

	const sql = `SELECT ` + userColumns + ` FROM users WHERE id = $1`

	return p.getUser(ctx, sql, id)
}
