package postgres

import (
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/vbloher/canton-private-otc-desk/pkg/transaction"
)

type Postgres struct {
	tx *transaction.Manager
}

func New(tx *transaction.Manager) *Postgres {
	return &Postgres{tx: tx}
}

func nullText(s string) pgtype.Text {
	return pgtype.Text{String: s, Valid: s != ""}
}
