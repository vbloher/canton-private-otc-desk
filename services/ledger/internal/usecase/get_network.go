package usecase

import (
	"context"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/dto"
)

func (u *UseCase) GetNetwork(ctx context.Context) dto.GetNetworkOutput {
	_, span := tracer.Start(ctx, "usecase GetNetwork")
	defer span.End()

	output := dto.GetNetworkOutput{Network: u.config.Network}

	if u.faucet == "" {
		return output
	}

	for _, inst := range u.config.Instruments {
		if amount, ok := u.config.FaucetAmounts[inst.Symbol]; ok {
			output.Faucet = append(output.Faucet, dto.Drip{Symbol: inst.Symbol, Amount: amount})
		}
	}

	return output
}
