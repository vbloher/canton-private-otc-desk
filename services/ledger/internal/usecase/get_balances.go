package usecase

import (
	"context"
	"fmt"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/dto"
)

func (u *UseCase) GetBalances(ctx context.Context, input dto.GetBalancesInput) (dto.GetBalancesOutput, error) {
	ctx, span := tracer.Start(ctx, "usecase GetBalances")
	defer span.End()

	var output dto.GetBalancesOutput

	holdings, err := u.canton.Holdings(ctx, input.PartyID)
	if err != nil {
		return output, fmt.Errorf("canton.Holdings: %w", err)
	}

	balances := domain.Balances(holdings, input.PartyID, u.operator, u.config.Instruments, locksAsOf())

	return dto.GetBalancesOutput{Balances: balances}, nil
}
