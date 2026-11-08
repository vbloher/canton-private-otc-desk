package postgres

import (
	"context"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/notification/internal/domain"
)

func (p *Postgres) GetProfileForUpdate(ctx context.Context, partyID string) (domain.Profile, error) {
	ctx, span := tracer.Start(ctx, "adapter postgres GetProfileForUpdate")
	defer span.End()

	return p.oneProfile(ctx, `SELECT `+profileColumns+` FROM profiles WHERE party_id = $1 FOR UPDATE`, partyID)
}
