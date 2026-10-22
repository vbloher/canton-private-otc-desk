package usecase_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/dto"
)

func allocation(cid, settlementID, authorizer, instrument string) domain.Allocation {
	return domain.Allocation{
		ContractID:    cid,
		SettlementID:  settlementID,
		Authorizer:    authorizer,
		Admin:         operator,
		InstrumentIDs: []string{instrument},
	}
}

func Test_GetSettlements_ReportsPerParty(t *testing.T) {
	d := newDeps(t)
	u := d.started(t)

	// One read of the executor's allocations answers every settlement of the batch.
	d.canton.EXPECT().Allocations(Any, operator).Return([]domain.Allocation{
		allocation("a1", "s-1", makerA, "CBTC"),
		allocation("a2", "s-1", makerA, "USDCx"),
		allocation("a3", "s-1", alice, "CBTC"),
		allocation("a4", "s-1", operator, "USDCx"),
		allocation("b1", "s-2", makerB, "CBTC"),
		allocation("b2", "s-2", makerB, "USDCx"),
	}, nil).Once()

	output, err := u.GetSettlements(t.Context(), dto.GetSettlementsInput{Settlements: []dto.SettlementQuery{
		{SettlementID: "s-1", Legs: tradeLegs(makerA, alice)},
		{SettlementID: "s-2", Legs: tradeLegs(makerB, alice)},
	}})
	require.NoError(t, err)

	require.Equal(t, []dto.SettlementStatus{
		{SettlementID: "s-1", Parties: []dto.PartyAllocation{
			{PartyID: alice, Allocated: false},
			{PartyID: makerA, Allocated: true},
			{PartyID: operator, Allocated: true},
		}},
		{SettlementID: "s-2", Parties: []dto.PartyAllocation{
			{PartyID: alice, Allocated: false},
			{PartyID: makerB, Allocated: true},
			{PartyID: operator, Allocated: false},
		}},
	}, output.Settlements)
}
