package ledger

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/vbloher/canton-private-otc-desk/gen/grpc/ledger_v1"
	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/trading/internal/domain"
)

func (l *Ledger) Settlements(
	ctx context.Context, queries map[uuid.UUID][]domain.Leg,
) (map[uuid.UUID]map[string]bool, error) {
	ctx, span := tracer.Start(ctx, "adapter ledger Settlements")
	defer span.End()

	in := &ledger_v1.GetSettlementsInput{Settlements: make([]*ledger_v1.SettlementQuery, 0, len(queries))}

	for id, legs := range queries {
		in.Settlements = append(in.Settlements, &ledger_v1.SettlementQuery{
			SettlementId: id.String(),
			Legs:         toLegs(legs),
		})
	}

	out, err := l.client.GetSettlements(ctx, in)
	if err != nil {
		return nil, wrap("GetSettlements", err)
	}

	result := make(map[uuid.UUID]map[string]bool, len(out.GetSettlements()))

	for _, s := range out.GetSettlements() {
		id, err := uuid.Parse(s.GetSettlementId())
		if err != nil {
			return nil, fmt.Errorf("%w: ledger GetSettlements: settlement id: %w", domain.ErrUpstream, err)
		}

		parties := make(map[string]bool, len(s.GetParties()))
		for _, p := range s.GetParties() {
			parties[p.GetPartyId()] = p.GetAllocated()
		}

		result[id] = parties
	}

	return result, nil
}
