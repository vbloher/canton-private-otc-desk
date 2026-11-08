package postgres

import (
	"context"
	"fmt"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/notification/internal/domain"
)

func (p *Postgres) ListProfiles(ctx context.Context, partyIDs []string) ([]domain.Profile, error) {
	ctx, span := tracer.Start(ctx, "adapter postgres ListProfiles")
	defer span.End()

	const sql = `SELECT ` + profileColumns + ` FROM profiles WHERE party_id = ANY($1)`

	rows, err := p.tx.TryExtractTX(ctx).Query(ctx, sql, partyIDs)
	if err != nil {
		return nil, fmt.Errorf("txOrPool.Query: %w", err)
	}
	defer rows.Close()

	var profiles []domain.Profile

	for rows.Next() {
		profile, err := scanProfile(rows)
		if err != nil {
			return nil, fmt.Errorf("scanProfile: %w", err)
		}

		profiles = append(profiles, profile)
	}

	err = rows.Err()
	if err != nil {
		return nil, fmt.Errorf("rows.Err: %w", err)
	}

	return profiles, nil
}
