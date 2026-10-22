package postgres

import "github.com/vbloher/canton-private-otc-desk/pkg/transaction"

type Postgres struct {
	tx *transaction.Manager
}

func New(tx *transaction.Manager) *Postgres {
	return &Postgres{
		tx: tx,
	}
}
