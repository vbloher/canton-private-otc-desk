package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/vbloher/canton-private-otc-desk/services/notification/internal/domain"
)

const profileColumns = `party_id, email, confirmed_at, news, trades`

func scanProfile(row pgx.Row) (domain.Profile, error) {
	var (
		profile domain.Profile
		email   pgtype.Text
	)

	err := row.Scan(&profile.PartyID, &email, &profile.ConfirmedAt, &profile.News, &profile.Trades)
	if err != nil {
		return domain.Profile{}, fmt.Errorf("row.Scan: %w", err)
	}

	profile.Email = email.String

	return profile, nil
}

func (p *Postgres) oneProfile(ctx context.Context, sql, partyID string) (domain.Profile, error) {
	profile, err := scanProfile(p.tx.TryExtractTX(ctx).QueryRow(ctx, sql, partyID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Profile{}, domain.ErrNotFound
		}

		return domain.Profile{}, fmt.Errorf("scanProfile: %w", err)
	}

	return profile, nil
}
