package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/domain"
)

const userColumns = `id, party_id, hint, display_name, kind, public_key, credential_id, encrypted_blob, api_key_hash,
                    maker_mode, created_at`

// scanUser reads one row of userColumns.
func scanUser(row pgx.Row) (domain.User, error) {
	var (
		user          domain.User
		kind          string
		credentialID  pgtype.Text
		encryptedBlob pgtype.Text
		apiKeyHash    pgtype.Text
	)

	dest := []any{
		&user.ID,
		&user.PartyID,
		&user.Hint,
		&user.DisplayName,
		&kind,
		&user.PublicKey,
		&credentialID,
		&encryptedBlob,
		&apiKeyHash,
		&user.MakerMode,
		&user.CreatedAt,
	}

	err := row.Scan(dest...)
	if err != nil {
		return domain.User{}, fmt.Errorf("row.Scan: %w", err)
	}

	user.Kind = domain.Kind(kind)
	user.CredentialID = credentialID.String
	user.EncryptedBlob = encryptedBlob.String
	user.APIKeyHash = apiKeyHash.String
	user.CreatedAt = user.CreatedAt.UTC()

	return user, nil
}

// getUser runs a query that selects userColumns of at most one user.
func (p *Postgres) getUser(ctx context.Context, sql string, args ...any) (domain.User, error) {
	txOrPool := p.tx.TryExtractTX(ctx)

	user, err := scanUser(txOrPool.QueryRow(ctx, sql, args...))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.User{}, domain.ErrNotFound
		}

		return domain.User{}, fmt.Errorf("scanUser: %w", err)
	}

	return user, nil
}

// nullText stores an absent value as NULL, which the unique constraints on these columns need.
func nullText(s string) pgtype.Text {
	return pgtype.Text{String: s, Valid: s != ""}
}
