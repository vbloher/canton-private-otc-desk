package ledger

import (
	"context"

	"github.com/google/uuid"

	"github.com/vbloher/canton-private-otc-desk/gen/grpc/ledger_v1"
	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
)

// CancelSettlement cancels every allocation of the settlement that is still active.
func (l *Ledger) CancelSettlement(ctx context.Context, settlementID uuid.UUID) error {
	ctx, span := tracer.Start(ctx, "adapter ledger CancelSettlement")
	defer span.End()

	_, err := l.client.CancelSettlement(ctx, &ledger_v1.CancelSettlementInput{SettlementId: settlementID.String()})
	if err != nil {
		return wrap("CancelSettlement", err)
	}

	return nil
}
