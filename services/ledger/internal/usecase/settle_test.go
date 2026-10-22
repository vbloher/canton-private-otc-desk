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

//nolint:gochecknoglobals
var tokens = []domain.Disclosed{{TemplateID: "pkg:Token", ContractID: "token-1", CreatedEventBlob: "blob-1"}}

func settledAllocations() []domain.Allocation {
	return []domain.Allocation{
		allocation("a-alice-cbtc", "s-1", alice, "CBTC"),
		allocation("a-maker-usdcx", "s-1", makerA, "USDCx"),
		allocation("a-alice-usdcx", "s-1", alice, "USDCx"),
		allocation("a-maker-cbtc", "s-1", makerA, "CBTC"),
		allocation("a-op-usdcx", "s-1", operator, "USDCx"),
		allocation("stray", "s-1", makerA, "CETH"),
	}
}

func Test_Settle_DuplicateCommandIsSuccess(t *testing.T) {
	d := newDeps(t)
	u := d.started(t)

	d.canton.EXPECT().Allocations(Any, operator).Return(settledAllocations(), nil).Once()
	d.cachedRules()
	d.canton.EXPECT().Tokens(Any, operator).Return(tokens, nil).Once()
	d.canton.EXPECT().SettleCommand(rules, tokens, mock.MatchedBy(func(s domain.SettleSpec) bool {
		// The plan-matched set, in plan order; the stray never joins the batch.
		return s.Operator == operator && s.SettlementID == "s-1" && len(s.Legs) == 3 &&
			slices.Equal(s.AllocationCIDs, []string{"a-alice-cbtc", "a-alice-usdcx", "a-maker-cbtc", "a-maker-usdcx", "a-op-usdcx"})
	})).Return(command("settle"), nil).Once()
	d.canton.EXPECT().Submit(Any, operator, "settle:s-1", command("settle")).
		Return(domain.Transaction{}, fmt.Errorf("%w: %w: DUPLICATE_COMMAND", domain.ErrLedger, domain.ErrDuplicateCommand)).Once()
	d.events.EXPECT().Publish(Any, eventOfType(events.WalletUpdated), parties(alice, makerA, operator)).Return(nil).Once()

	output, err := u.Settle(t.Context(), dto.SettleInput{SettlementID: "s-1", Legs: tradeLegs(makerA, alice)})
	require.NoError(t, err)
	require.Empty(t, output.UpdateID)
}

func Test_Settle_ReturnsUpdateID(t *testing.T) {
	d := newDeps(t)
	u := d.started(t)

	d.canton.EXPECT().Allocations(Any, operator).Return(settledAllocations(), nil).Once()
	d.cachedRules()
	d.canton.EXPECT().Tokens(Any, operator).Return(tokens, nil).Once()
	d.canton.EXPECT().SettleCommand(rules, tokens, Any).Return(command("settle"), nil).Once()
	d.canton.EXPECT().Submit(Any, operator, "settle:s-1", command("settle")).
		Return(domain.Transaction{UpdateID: "update-1"}, nil).Once()
	d.events.EXPECT().Publish(Any, eventOfType(events.WalletUpdated), Any).Return(nil).Once()

	output, err := u.Settle(t.Context(), dto.SettleInput{SettlementID: "s-1", Legs: tradeLegs(makerA, alice)})
	require.NoError(t, err)
	require.Equal(t, "update-1", output.UpdateID)
}

func Test_Settle_MissingAllocation(t *testing.T) {
	d := newDeps(t)
	u := d.started(t)

	d.canton.EXPECT().Allocations(Any, operator).Return(settledAllocations()[:3], nil).Once()

	_, err := u.Settle(t.Context(), dto.SettleInput{SettlementID: "s-1", Legs: tradeLegs(makerA, alice)})
	require.ErrorIs(t, err, domain.ErrNotFound)
}

func Test_CancelSettlement_GoneCountsAsCancelled(t *testing.T) {
	d := newDeps(t)
	u := d.started(t)

	d.postgres.EXPECT().ExpireActionsByRef(Any, "s-1").Return(nil, nil).Once()
	d.canton.EXPECT().Allocations(Any, operator).Return([]domain.Allocation{
		allocation("a1", "s-1", makerA, "CBTC"),
		allocation("a2", "s-1", makerA, "USDCx"),
		allocation("other", "s-2", makerA, "CBTC"),
	}, nil).Once()
	d.cachedRules()
	d.canton.EXPECT().Tokens(Any, operator).Return(tokens, nil).Once()
	d.canton.EXPECT().CancelCommand(rules, tokens, "a1", operator).Return(command("cancel-a1"), nil).Once()
	d.canton.EXPECT().CancelCommand(rules, tokens, "a2", operator).Return(command("cancel-a2"), nil).Once()
	d.canton.EXPECT().Submit(Any, operator, "cancel:a1", command("cancel-a1")).Return(domain.Transaction{}, nil).Once()
	d.canton.EXPECT().Submit(Any, operator, "cancel:a2", command("cancel-a2")).
		Return(domain.Transaction{}, fmt.Errorf("%w: %w: CONTRACT_NOT_FOUND", domain.ErrLedger, domain.ErrContractGone)).Once()
	d.events.EXPECT().Publish(Any, eventOfType(events.WalletUpdated), parties(makerA)).Return(nil).Once()

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

	// An allocate action parked in the sign queue must never execute after the cancel: it
	// would leave a committed allocation of a dead settlement behind.
	parked := pendingAllocation(alice, "s-1", "Allocate 100000.0 USDCx for quote s-1")

	expire := d.postgres.EXPECT().ExpireActionsByRef(Any, "s-1").Return([]domain.Action{parked}, nil).Once()
	d.events.EXPECT().Publish(Any, event(events.SignCompleted, fmt.Sprintf(`{"actionId":%q}`, parked.ID)),
		parties(alice)).Return(nil).Once()
	d.canton.EXPECT().Allocations(Any, operator).Return([]domain.Allocation{
		allocation("a1", "s-1", makerA, "CBTC"),
	}, nil).Once().NotBefore(expire)
	d.cachedRules()
	d.canton.EXPECT().Tokens(Any, operator).Return(tokens, nil).Once()
	d.canton.EXPECT().CancelCommand(rules, tokens, "a1", operator).Return(command("cancel-a1"), nil).Once()
	d.canton.EXPECT().Submit(Any, operator, "cancel:a1", command("cancel-a1")).Return(domain.Transaction{}, nil).Once()
	d.events.EXPECT().Publish(Any, eventOfType(events.WalletUpdated), parties(makerA)).Return(nil).Once()

	output, err := u.CancelSettlement(t.Context(), dto.CancelSettlementInput{SettlementID: "s-1"})
	require.NoError(t, err)
	require.Equal(t, 1, output.Cancelled)
}
