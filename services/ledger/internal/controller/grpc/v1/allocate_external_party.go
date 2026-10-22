package v1

import (
	"context"

	pb "github.com/vbloher/canton-private-otc-desk/gen/grpc/ledger_v1"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/dto"
)

func (h Handlers) AllocateExternalParty(
	ctx context.Context, i *pb.AllocateExternalPartyInput,
) (*pb.AllocateExternalPartyOutput, error) {
	output, err := h.usecase.AllocateExternalParty(ctx, dto.AllocateExternalPartyInput{
		Transactions: i.GetTopologyTransactions(),
		Signature:    i.GetSignature(),
		Fingerprint:  i.GetFingerprint(),
	})
	if err != nil {
		return nil, grpcError(ctx, err)
	}

	return &pb.AllocateExternalPartyOutput{PartyId: output.PartyID}, nil
}
