package ledger

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/vbloher/canton-private-otc-desk/gen/grpc/ledger_v1"
	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/trading/internal/domain"
)

func (l *Ledger) Settle(ctx context.Context, settlementID uuid.UUID, legs []domain.Leg) (string, error) {
	ctx, span := tracer.Start(ctx, "adapter ledger Settle")
	defer span.End()

	out, err := l.client.Settle(ctx, &ledger_v1.SettleInput{
		SettlementId: settlementID.String(),
		Legs:         toLegs(legs),
	})
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return "", fmt.Errorf("%w: ledger Settle: %w", domain.ErrSettlementGone, err)
		}

		return "", wrap("Settle", err)
	}

	return out.GetUpdateId(), nil
}
