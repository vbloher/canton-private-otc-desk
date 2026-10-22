package ledger

import (
	"context"
	"time"

	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/vbloher/canton-private-otc-desk/gen/grpc/ledger_v1"
	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/trading/internal/domain"
)

// Allocate creates (hosted party) or queues for signature (external party) every allocation the
// party owes to the settlement.
func (l *Ledger) Allocate(
	ctx context.Context, settlementID uuid.UUID, deadline time.Time, legs []domain.Leg, party, description string,
) error {
	ctx, span := tracer.Start(ctx, "adapter ledger Allocate")
	defer span.End()

	_, err := l.client.Allocate(ctx, &ledger_v1.AllocateInput{
		SettlementId: settlementID.String(),
		Deadline:     timestamppb.New(deadline),
		Legs:         toLegs(legs),
		PartyId:      party,
		Description:  description,
	})
	if err != nil {
		return wrap("Allocate", err)
	}

	return nil
}
