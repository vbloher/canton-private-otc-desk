package postgres

import (
	"context"
	"fmt"

	"github.com/doug-martin/goqu/v9"

	_ "github.com/doug-martin/goqu/v9/dialect/postgres"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/domain"
)

func (p *Postgres) SaveActions(ctx context.Context, actions ...domain.Action) error {
	ctx, span := tracer.Start(ctx, "adapter postgres SaveActions")
	defer span.End()

	if len(actions) == 0 {
		return nil
	}

	rows := make([]any, 0, len(actions))

	for _, a := range actions {
		rows = append(rows, goqu.Record{
			"id":                     a.ID,
			"party_id":               a.PartyID,
			"purpose":                a.Purpose,
			"description":            a.Description,
			"ref":                    a.Ref,
			"counterparty":           a.Counterparty,
			"hash":                   a.Hash,
			"prepared_transaction":   a.Prepared.Transaction,
			"hashing_scheme_version": a.Prepared.HashingSchemeVersion,
			"status":                 string(a.Status),
			"error":                  a.Error,
			"created_at":             a.CreatedAt,
			"expires_at":             a.ExpiresAt,
		})
	}

	sql, args, err := goqu.Dialect("postgres").Insert("actions").Prepared(true).Rows(rows...).ToSQL()
	if err != nil {
		return fmt.Errorf("goqu.Insert.ToSQL: %w", err)
	}

	txOrPool := p.tx.TryExtractTX(ctx)

	_, err = txOrPool.Exec(ctx, sql, args...)
	if err != nil {
		return fmt.Errorf("txOrPool.Exec: %w", err)
	}

	return nil
}
