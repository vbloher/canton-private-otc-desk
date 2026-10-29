package postgres

import (
	"context"
	"fmt"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/domain"
)

func (p *Postgres) UpsertHouseUser(ctx context.Context, user domain.User) error {
	ctx, span := tracer.Start(ctx, "adapter postgres UpsertHouseUser")
	defer span.End()

	const sql = `INSERT INTO users (id, party_id, hint, display_name, kind, api_key_hash, maker_mode, created_at)
                    VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
                    ON CONFLICT (hint) DO UPDATE
                    SET party_id = EXCLUDED.party_id,
                        display_name = EXCLUDED.display_name,
                        api_key_hash = EXCLUDED.api_key_hash,
                        maker_mode = EXCLUDED.maker_mode
                    WHERE users.kind = 'house'`

	args := []any{
		user.ID,
		user.PartyID,
		user.Hint,
		user.DisplayName,
		string(user.Kind),
		nullText(user.APIKeyHash),
		user.MakerMode,
		user.CreatedAt,
	}

	txOrPool := p.tx.TryExtractTX(ctx)

	tag, err := txOrPool.Exec(ctx, sql, args...)
	if err != nil {
		return fmt.Errorf("txOrPool.Exec: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return fmt.Errorf("hint %q belongs to a wallet, not to a house maker", user.Hint)
	}

	return nil
}
