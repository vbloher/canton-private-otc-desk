package outbox

import (
	"context"
	"errors"
	"fmt"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/pkg/transaction"
)

var ErrNoTransaction = errors.New("outbox: a message must be added inside a transaction")

type Writer struct {
	tx *transaction.Manager
}

func NewWriter(tx *transaction.Manager) *Writer {
	return &Writer{tx: tx}
}

func (w *Writer) Add(ctx context.Context, m Message) error {
	if !w.tx.InTransaction(ctx) {
		return ErrNoTransaction
	}

	const sql = `INSERT INTO outbox (event_id, topic, key, type, payload, headers) VALUES ($1, $2, $3, $4, $5, $6)`

	headers := map[string]string{}
	tracer.Inject(ctx, headers)

	_, err := w.tx.TryExtractTX(ctx).Exec(ctx, sql, m.ID, m.Topic, m.Key, m.Type, m.Payload, headers)
	if err != nil {
		return fmt.Errorf("txOrPool.Exec: %w", err)
	}

	return nil
}
