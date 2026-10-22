package identity

import (
	"context"
	"fmt"

	"github.com/vbloher/canton-private-otc-desk/gen/grpc/identity_v1"
	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/trading/internal/domain"
)

// ResolveMakers returns the users in maker mode among the hints; unknown hints are left out.
func (i *Identity) ResolveMakers(ctx context.Context, hints []string) ([]domain.Party, error) {
	ctx, span := tracer.Start(ctx, "adapter identity ResolveMakers")
	defer span.End()

	out, err := i.client.ResolveMakers(ctx, &identity_v1.ResolveMakersInput{Hints: hints})
	if err != nil {
		return nil, fmt.Errorf("%w: identity ResolveMakers: %w", domain.ErrUpstream, err)
	}

	makers := make([]domain.Party, 0, len(out.GetMakers()))

	for _, u := range out.GetMakers() {
		if u.GetMakerMode() {
			makers = append(makers, domain.Party{ID: u.GetPartyId(), Hint: u.GetHint()})
		}
	}

	return makers, nil
}
