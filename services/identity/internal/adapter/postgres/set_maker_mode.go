package postgres

import (
	"context"

	"github.com/google/uuid"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/domain"
)

func (p *Postgres) SetMakerMode(ctx context.Context, id uuid.UUID, enabled bool) (domain.User, error) {
	ctx, span := tracer.Start(ctx, "adapter postgres SetMakerMode")
	defer span.End()

	const sql = `UPDATE users SET maker_mode = $2 WHERE id = $1 RETURNING ` + userColumns

	return p.getUser(ctx, sql, id, enabled)
}
