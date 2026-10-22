package ledger

import (
	"context"
	"fmt"

	pb "github.com/vbloher/canton-private-otc-desk/gen/grpc/ledger_v1"
	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/domain"
)

func (l *Ledger) AllocateExternalParty(
	ctx context.Context, topology domain.Topology, signature []byte,
) (string, error) {
	ctx, span := tracer.Start(ctx, "adapter ledger AllocateExternalParty")
	defer span.End()

	output, err := l.client.AllocateExternalParty(ctx, &pb.AllocateExternalPartyInput{
		TopologyTransactions: topology.Transactions,
		Signature:            signature,
		Fingerprint:          topology.Fingerprint,
	})
	if err != nil {
		return "", mapError(err)
	}

	if output.GetPartyId() == "" {
		return "", fmt.Errorf("%w: no party id for %q", domain.ErrLedger, topology.PartyID)
	}

	return output.GetPartyId(), nil
}
