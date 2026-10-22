package ledger

import (
	"context"
	"fmt"

	pb "github.com/vbloher/canton-private-otc-desk/gen/grpc/ledger_v1"
	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/domain"
)

func (l *Ledger) ResolveParty(ctx context.Context, hint string) (string, error) {
	ctx, span := tracer.Start(ctx, "adapter ledger ResolveParty")
	defer span.End()

	output, err := l.client.ResolveParty(ctx, &pb.ResolvePartyInput{Hint: hint})
	if err != nil {
		return "", mapError(err)
	}

	if output.GetPartyId() == "" {
		return "", fmt.Errorf("%w: no party id for %q", domain.ErrLedger, hint)
	}

	return output.GetPartyId(), nil
}
