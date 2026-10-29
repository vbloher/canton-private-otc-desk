package usecase_test

import (
	"fmt"
	"slices"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/pkg/events"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/dto"
)

func settledAllocations() []domain.Allocation {
	return []domain.Allocation{
		allocation("a-alice-cbtc", "s-1", alice, "CBTC"),
		allocation("a-maker-usdcx", "s-1", maker, "USDCx"),
		allocation("a-alice-usdcx", "s-1", alice, "USDCx"),
		allocation("a-maker-cbtc", "s-1", maker, "CBTC"),
		allocation("a-op-usdcx", "s-1", operator, "USDCx"),
		allocation("stray", "s-1", maker, "CETH"),
	}
}

func Test_Settle_DuplicateCommandIsSuccess(t *testing.T) {
	d := newDeps(t)
	u := d.started(t)

	d.canton.EXPECT().Allocations(Any, operator).Return(settledAllocations(), nil).Once()
	d.registry.EXPECT().SettleCommand(Any, mock.MatchedBy(func(s domain.SettleSpec) bool {
		return s.Operator == operator && s.SettlementID == "s-1" && len(s.Batches) == 1 &&
			s.Batches[0].Admin == operator && s.Batches[0].Registry == registryURL && len(s.Batches[0].Legs) == 3 &&
			slices.Equal(s.Batches[0].AllocationCIDs,
				[]string{"a-alice-cbtc", "a-alice-usdcx", "a-maker-cbtc", "a-maker-usdcx", "a-op-usdcx"})
	})).Return(command("settle"), nil).Once()
	d.canton.EXPECT().Submit(Any, operator, "settle:s-1", command("settle")).
		Return(domain.Transaction{}, fmt.Errorf("%w: %w: DUPLICATE_COMMAND", domain.ErrLedger, domain.ErrDuplicateCommand)).Once()
	d.events.EXPECT().Publish(Any, eventOfType(events.WalletUpdated), parties(alice, maker, operator)).Return(nil).Once()

	output, err := u.Settle(t.Context(), dto.SettleInput{SettlementID: "s-1", Legs: tradeLegs(maker, alice)})
	require.NoError(t, err)
	require.Empty(t, output.UpdateID)
}

func Test_Settle_ReturnsUpdateID(t *testing.T) {
	d := newDeps(t)
	u := d.started(t)

	d.canton.EXPECT().Allocations(Any, operator).Return(settledAllocations(), nil).Once()
	d.registry.EXPECT().SettleCommand(Any, Any).Return(command("settle"), nil).Once()
	d.canton.EXPECT().Submit(Any, operator, "settle:s-1", command("settle")).
		Return(domain.Transaction{UpdateID: "update-1"}, nil).Once()
	d.events.EXPECT().Publish(Any, eventOfType(events.WalletUpdated), Any).Return(nil).Once()

	output, err := u.Settle(t.Context(), dto.SettleInput{SettlementID: "s-1", Legs: tradeLegs(maker, alice)})
	require.NoError(t, err)
	require.Equal(t, "update-1", output.UpdateID)
}

func Test_Settle_MissingAllocation(t *testing.T) {
	d := newDeps(t)
	u := d.started(t)

	d.canton.EXPECT().Allocations(Any, operator).Return(settledAllocations()[:3], nil).Once()

	_, err := u.Settle(t.Context(), dto.SettleInput{SettlementID: "s-1", Legs: tradeLegs(maker, alice)})
	require.ErrorIs(t, err, domain.ErrNotFound)
}

func Test_CancelSettlement_GoneCountsAsCancelled(t *testing.T) {
	d := newDeps(t)
	u := d.started(t)

	d.postgres.EXPECT().ExpireActionsByRef(Any, "s-1").Return(nil, nil).Once()
	d.canton.EXPECT().Allocations(Any, operator).Return([]domain.Allocation{
		allocation("a1", "s-1", maker, "CBTC"),
		allocation("a2", "s-1", maker, "USDCx"),
		allocation("other", "s-2", maker, "CBTC"),
	}, nil).Once()
	d.registry.EXPECT().CancelCommand(Any, registryURL, "a1", operator).Return(command("cancel-a1"), nil).Once()
	d.registry.EXPECT().CancelCommand(Any, registryURL, "a2", operator).Return(command("cancel-a2"), nil).Once()
	d.canton.EXPECT().Submit(Any, operator, "cancel:a1", command("cancel-a1")).Return(domain.Transaction{}, nil).Once()
	d.canton.EXPECT().Submit(Any, operator, "cancel:a2", command("cancel-a2")).
		Return(domain.Transaction{}, fmt.Errorf("%w: %w: CONTRACT_NOT_FOUND", domain.ErrLedger, domain.ErrContractGone)).Once()
	d.events.EXPECT().Publish(Any, eventOfType(events.WalletUpdated), parties(maker)).Return(nil).Once()

	output, err := u.CancelSettlement(t.Context(), dto.CancelSettlementInput{SettlementID: "s-1"})
	require.NoError(t, err)
	require.Equal(t, 2, output.Cancelled)
}

func Test_CancelSettlement_NothingToCancel(t *testing.T) {
	d := newDeps(t)
	u := d.started(t)

	d.postgres.EXPECT().ExpireActionsByRef(Any, "s-1").Return(nil, nil).Once()
	d.canton.EXPECT().Allocations(Any, operator).Return(nil, nil).Once()

	output, err := u.CancelSettlement(t.Context(), dto.CancelSettlementInput{SettlementID: "s-1"})
	require.NoError(t, err)
	require.Zero(t, output.Cancelled)
}

func Test_CancelSettlement_ExpiresPendingActions(t *testing.T) {
	d := newDeps(t)
	u := d.started(t)

	parked := pendingAllocation(alice, "s-1", "Allocate 100000.0 USDCx for quote s-1")

	expire := d.postgres.EXPECT().ExpireActionsByRef(Any, "s-1").Return([]domain.Action{parked}, nil).Once()
	d.events.EXPECT().Publish(Any, event(events.SignCompleted, fmt.Sprintf(`{"actionId":%q}`, parked.ID)),
		parties(alice)).Return(nil).Once()
	d.canton.EXPECT().Allocations(Any, operator).Return([]domain.Allocation{
		allocation("a1", "s-1", maker, "CBTC"),
	}, nil).Once().NotBefore(expire)
	d.registry.EXPECT().CancelCommand(Any, registryURL, "a1", operator).Return(command("cancel-a1"), nil).Once()
	d.canton.EXPECT().Submit(Any, operator, "cancel:a1", command("cancel-a1")).Return(domain.Transaction{}, nil).Once()
	d.events.EXPECT().Publish(Any, eventOfType(events.WalletUpdated), parties(maker)).Return(nil).Once()

	output, err := u.CancelSettlement(t.Context(), dto.CancelSettlementInput{SettlementID: "s-1"})
	require.NoError(t, err)
	require.Equal(t, 1, output.Cancelled)
}

func Test_Settle_OneSubmissionAcrossAdmins(t *testing.T) {
	d := newDeps(t)
	u := d.started(t)

	legs := []dto.TransferLeg{
		{ID: "base", Sender: maker, Receiver: alice, Instrument: "CBTC", Amount: "1.0"},
		{ID: "quote", Sender: alice, Receiver: maker, Instrument: "CC", Amount: "500000.0"},
	}
	amulet := func(cid, authorizer string) domain.Allocation {
		a := allocation(cid, "s-1", authorizer, "Amulet")
		a.Admin = dso

		return a
	}

	d.canton.EXPECT().Allocations(Any, operator).Return([]domain.Allocation{
		allocation("a-maker-cbtc", "s-1", maker, "CBTC"),
		allocation("a-alice-cbtc", "s-1", alice, "CBTC"),
		amulet("a-maker-cc", maker),
		amulet("a-alice-cc", alice),
	}, nil).Once()
	d.registry.EXPECT().SettleCommand(Any, mock.MatchedBy(func(s domain.SettleSpec) bool {
		return len(s.Batches) == 2 &&
			s.Batches[0].Admin == operator && s.Batches[0].Registry == registryURL &&
			slices.Equal(s.Batches[0].AllocationCIDs, []string{"a-alice-cbtc", "a-maker-cbtc"}) &&
			s.Batches[1].Admin == dso && s.Batches[1].Registry == scanURL &&
			s.Batches[1].Legs[0].Instrument.ID == "Amulet" &&
			slices.Equal(s.Batches[1].AllocationCIDs, []string{"a-alice-cc", "a-maker-cc"})
	})).Return(command("settle"), nil).Once()
	d.canton.EXPECT().Submit(Any, operator, "settle:s-1", command("settle")).
		Return(domain.Transaction{UpdateID: "update-1"}, nil).Once()
	d.events.EXPECT().Publish(Any, eventOfType(events.WalletUpdated), parties(alice, maker)).Return(nil).Once()

	output, err := u.Settle(t.Context(), dto.SettleInput{SettlementID: "s-1", Legs: legs})
	require.NoError(t, err)
	require.Equal(t, "update-1", output.UpdateID)
}

func Test_CancelSettlement_UsesTheAdminsRegistry(t *testing.T) {
	d := newDeps(t)
	u := d.started(t)

	cc := allocation("a-cc", "s-1", alice, "Amulet")
	cc.Admin = dso
	gone := allocation("a-gone", "s-1", maker, "CBTC")

	d.postgres.EXPECT().ExpireActionsByRef(Any, "s-1").Return(nil, nil).Once()
	d.canton.EXPECT().Allocations(Any, operator).Return([]domain.Allocation{cc, gone}, nil).Once()
	d.registry.EXPECT().CancelCommand(Any, scanURL, "a-cc", operator).Return(command("cancel-cc"), nil).Once()
	d.registry.EXPECT().CancelCommand(Any, registryURL, "a-gone", operator).
		Return(domain.Command{}, fmt.Errorf("%w: %w: HTTP 404", domain.ErrLedger, domain.ErrContractGone)).Once()
	d.canton.EXPECT().Submit(Any, operator, "cancel:a-cc", command("cancel-cc")).Return(domain.Transaction{}, nil).Once()
	d.events.EXPECT().Publish(Any, eventOfType(events.WalletUpdated), parties(alice, maker)).Return(nil).Once()

	output, err := u.CancelSettlement(t.Context(), dto.CancelSettlementInput{SettlementID: "s-1"})
	require.NoError(t, err)
	require.Equal(t, 2, output.Cancelled)
}
