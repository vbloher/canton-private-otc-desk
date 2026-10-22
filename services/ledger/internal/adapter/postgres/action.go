package postgres

import (
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/domain"
)

const actionColumns = `id, party_id, purpose, description, ref, counterparty, hash, prepared_transaction,
                    hashing_scheme_version, status, error, created_at, expires_at`

func scanAction(row pgx.Row) (domain.Action, error) {
	var (
		a      domain.Action
		status string
	)

	err := row.Scan(&a.ID, &a.PartyID, &a.Purpose, &a.Description, &a.Ref, &a.Counterparty, &a.Hash,
		&a.Prepared.Transaction, &a.Prepared.HashingSchemeVersion, &status, &a.Error, &a.CreatedAt, &a.ExpiresAt)
	if err != nil {
		return domain.Action{}, fmt.Errorf("row.Scan: %w", err)
	}

	a.Status = domain.ActionStatus(status)
	a.Prepared.Hash = a.Hash
	a.CreatedAt = a.CreatedAt.UTC()
	a.ExpiresAt = a.ExpiresAt.UTC()

	return a, nil
}

func scanActions(rows pgx.Rows) ([]domain.Action, error) {
	defer rows.Close()

	var actions []domain.Action

	for rows.Next() {
		a, err := scanAction(rows)
		if err != nil {
			return nil, err
		}

		actions = append(actions, a)
	}

	err := rows.Err()
	if err != nil {
		return nil, fmt.Errorf("rows.Err: %w", err)
	}

	return actions, nil
}
