package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/domain"
)

const uniqueViolation = "23505"

func (p *Postgres) CreateUser(ctx context.Context, user domain.User) error {
	ctx, span := tracer.Start(ctx, "adapter postgres CreateUser")
	defer span.End()

	const sql = `INSERT INTO users (id, party_id, hint, display_name, kind, public_key, credential_id,
                                   encrypted_blob, api_key_hash, maker_mode, created_at)
                    VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`

	args := []any{
		user.ID,
		user.PartyID,
		user.Hint,
		user.DisplayName,
		string(user.Kind),
		user.PublicKey,
		nullText(user.CredentialID),
		nullText(user.EncryptedBlob),
		nullText(user.APIKeyHash),
		user.MakerMode,
		user.CreatedAt,
	}

	txOrPool := p.tx.TryExtractTX(ctx)

	_, err := txOrPool.Exec(ctx, sql, args...)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation && pgErr.ConstraintName == "users_credential_id_key" {
			return domain.ErrCredentialTaken
		}

		return fmt.Errorf("txOrPool.Exec: %w", err)
	}

	return nil
}
