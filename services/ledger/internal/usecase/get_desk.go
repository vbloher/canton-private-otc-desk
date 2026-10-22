package usecase

import (
	"context"
	"slices"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/dto"
)

func (u *UseCase) GetDesk(ctx context.Context) dto.GetDeskOutput {
	_, span := tracer.Start(ctx, "usecase GetDesk")
	defer span.End()

	return dto.GetDeskOutput{
		OperatorPartyID: u.operator,
		Instruments:     slices.Clone(u.config.Instruments),
	}
}
