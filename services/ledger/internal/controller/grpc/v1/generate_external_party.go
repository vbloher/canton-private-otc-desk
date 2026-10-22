package v1

import (
	"context"

	pb "github.com/vbloher/canton-private-otc-desk/gen/grpc/ledger_v1"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/dto"
)

func (h Handlers) GenerateExternalParty(
	ctx context.Context, i *pb.GenerateExternalPartyInput,
) (*pb.GenerateExternalPartyOutput, error) {
	output, err := h.usecase.GenerateExternalParty(ctx, dto.GenerateExternalPartyInput{
		Hint:      i.GetHint(),
		PublicKey: i.GetPublicKey(),
	})
	if err != nil {
		return nil, grpcError(ctx, err)
	}

	return &pb.GenerateExternalPartyOutput{
		PartyId:              output.PartyID,
		Fingerprint:          output.Fingerprint,
		MultiHash:            output.MultiHash,
		TopologyTransactions: output.Transactions,
	}, nil
}
