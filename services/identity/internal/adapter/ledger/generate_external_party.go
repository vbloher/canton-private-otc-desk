package ledger

import (
	"context"
	"fmt"

	pb "github.com/vbloher/canton-private-otc-desk/gen/grpc/ledger_v1"
	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/domain"
)

func (l *Ledger) GenerateExternalParty(
	ctx context.Context, hint string, publicKey []byte,
) (domain.Topology, error) {
	ctx, span := tracer.Start(ctx, "adapter ledger GenerateExternalParty")
	defer span.End()

	output, err := l.client.GenerateExternalParty(ctx, &pb.GenerateExternalPartyInput{
		Hint:      hint,
		PublicKey: publicKey,
	})
	if err != nil {
		return domain.Topology{}, mapError(err)
	}

	topology := domain.Topology{
		PartyID:      output.GetPartyId(),
		Fingerprint:  output.GetFingerprint(),
		MultiHash:    output.GetMultiHash(),
		Transactions: output.GetTopologyTransactions(),
	}

	if topology.PartyID == "" || topology.Fingerprint == "" ||
		len(topology.MultiHash) == 0 || len(topology.Transactions) == 0 {
		return domain.Topology{}, fmt.Errorf("%w: incomplete topology for %q", domain.ErrLedger, hint)
	}

	return topology, nil
}
