package usecase_test

import (
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/pkg/events"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/dto"
)

func allocateSpec(instrument string, inputs ...string) any {
	return mock.MatchedBy(func(s domain.AllocateSpec) bool {
		return s.Operator == operator && s.SettlementID == "s-1" && s.Slot.InstrumentID == instrument &&
			slices.Equal(s.Inputs, inputs) && s.Deadline.After(time.Now()) && !s.RequestedAt.After(time.Now())
	})
}

func Test_Allocate_HostedPartySubmits(t *testing.T) {
	d := newDeps(t)
	u := d.startedWithFaucet(t, faucetConfig(t))

	d.canton.EXPECT().Holdings(Any, faucet).Return([]domain.Holding{
		holding("h-cbtc", faucet, "CBTC", "10.0"),
		holding("h-usdcx", faucet, "USDCx", "1000000.0"),
	}, nil).Once()
	d.registry.EXPECT().AllocateCommand(Any, allocateSpec("CBTC", "h-cbtc")).Return(command("cbtc"), nil).Once()
	d.registry.EXPECT().AllocateCommand(Any, allocateSpec("USDCx", "h-usdcx")).Return(command("usdcx"), nil).Once()
	d.canton.EXPECT().Submit(Any, faucet, "allocate:s-1:"+faucet, commands("cbtc", "usdcx")).
		Return(domain.Transaction{UpdateID: "u-1"}, nil).Once()
	d.events.EXPECT().Publish(Any, event(events.WalletUpdated, `{}`), parties(faucet)).Return(nil).Once()

	output, err := u.Allocate(t.Context(), dto.AllocateInput{
		SettlementID: "s-1",
		Deadline:     time.Now().Add(time.Hour),
		Legs:         tradeLegs(faucet, alice),
		PartyID:      faucet,
		Description:  "quote s-1",
	})
	require.NoError(t, err)
	require.True(t, output.Executed)
	require.Empty(t, output.ActionIDs)
}

func Test_Allocate_ExternalPartyEnqueues(t *testing.T) {
	d := newDeps(t)
	u := d.started(t)

	d.postgres.EXPECT().ListPendingActions(Any, alice, Any).Return(nil, nil).Once()
	d.canton.EXPECT().Holdings(Any, alice).Return([]domain.Holding{
		holding("h-usdcx", alice, "USDCx", "150000.0"),
	}, nil).Once()
	d.registry.EXPECT().AllocateCommand(Any, allocateSpec("CBTC")).Return(command("cbtc"), nil).Once()
	d.registry.EXPECT().AllocateCommand(Any, allocateSpec("USDCx", "h-usdcx")).Return(command("usdcx"), nil).Once()
	d.canton.EXPECT().Prepare(Any, alice, "allocate:s-1:"+alice, commands("cbtc", "usdcx")).
		Return(prepared("hash-both"), nil).Once()

	var saved []domain.Action

	d.postgres.EXPECT().SaveActions(Any, mock.MatchedBy(func(actions []domain.Action) bool {
		saved = actions

		return len(actions) == 1
	})).Return(nil).Once()
	d.events.EXPECT().Publish(Any, eventOfType(events.SignRequested), parties(alice)).Return(nil).Once()

	output, err := u.Allocate(t.Context(), dto.AllocateInput{
		SettlementID: "s-1",
		Deadline:     time.Now().Add(time.Hour),
		Legs:         tradeLegs(maker, alice),
		PartyID:      alice,
		Description:  "quote s-1",
	})
	require.NoError(t, err)
	require.False(t, output.Executed)

	a := saved[0]
	require.Equal(t, domain.ActionPending, a.Status)
	require.Equal(t, alice, a.PartyID)
	require.Equal(t, domain.PurposeAllocate, a.Purpose)
	require.Equal(t, "s-1", a.Ref)
	require.Equal(t, "hash-both", string(a.Hash))
	require.Equal(t, a.Hash, a.Prepared.Hash)
	require.WithinDuration(t, time.Now().Add(10*time.Minute), a.ExpiresAt, time.Minute)
	require.Equal(t, "Allocate receipt of CBTC and 100000.0 USDCx for quote s-1", a.Description)
	require.Equal(t, []uuid.UUID{a.ID}, output.ActionIDs)
}

func Test_Allocate_OneTransactionPerAllocationAcrossSynchronizers(t *testing.T) {
	d := newDeps(t)
	u := d.started(t)

	cbtc := command("cbtc")
	cbtc.SynchronizerID = "private"
	usdcx := command("usdcx")
	usdcx.SynchronizerID = "global"

	d.postgres.EXPECT().ListPendingActions(Any, alice, Any).Return(nil, nil).Once()
	d.canton.EXPECT().Holdings(Any, alice).Return([]domain.Holding{
		holding("h-usdcx", alice, "USDCx", "150000.0"),
	}, nil).Once()
	d.registry.EXPECT().AllocateCommand(Any, allocateSpec("CBTC")).Return(cbtc, nil).Once()
	d.registry.EXPECT().AllocateCommand(Any, allocateSpec("USDCx", "h-usdcx")).Return(usdcx, nil).Once()
	d.canton.EXPECT().Prepare(Any, alice, "allocate:s-1:"+alice+":CBTC", cbtc).Return(prepared("hash-cbtc"), nil).Once()
	d.canton.EXPECT().Prepare(Any, alice, "allocate:s-1:"+alice+":USDCx", usdcx).Return(prepared("hash-usdcx"), nil).Once()

	var saved []domain.Action

	d.postgres.EXPECT().SaveActions(Any, mock.MatchedBy(func(actions []domain.Action) bool {
		saved = actions

		return len(actions) == 2
	})).Return(nil).Once()
	d.events.EXPECT().Publish(Any, eventOfType(events.SignRequested), parties(alice)).Return(nil).Times(2)

	_, err := u.Allocate(t.Context(), dto.AllocateInput{
		SettlementID: "s-1",
		Deadline:     time.Now().Add(time.Hour),
		Legs:         tradeLegs(maker, alice),
		PartyID:      alice,
		Description:  "quote s-1",
	})
	require.NoError(t, err)
	require.Equal(t, "Allocate receipt of CBTC for quote s-1", saved[0].Description)
	require.Equal(t, "Allocate 100000.0 USDCx for quote s-1", saved[1].Description)
}

func Test_Allocate_ExternalPartyGetsItsPendingActionsBack(t *testing.T) {
	d := newDeps(t)
	u := d.started(t)

	queued := []domain.Action{
		pendingAllocation(alice, "s-1", "Allocate receipt of CBTC for quote s-1"),
		pendingAllocation(alice, "s-1", "Allocate 100000.0 USDCx for quote s-1"),
	}
	other := pendingAllocation(alice, "s-2", "Allocate 1.0 CBTC for quote s-2")

	d.postgres.EXPECT().ListPendingActions(Any, alice, Any).Return(append(queued, other), nil).Once()

	output, err := u.Allocate(t.Context(), dto.AllocateInput{
		SettlementID: "s-1",
		Deadline:     time.Now().Add(time.Hour),
		Legs:         tradeLegs(maker, alice),
		PartyID:      alice,
	})
	require.NoError(t, err)
	require.False(t, output.Executed)
	require.Equal(t, []uuid.UUID{queued[0].ID, queued[1].ID}, output.ActionIDs)
}

func pendingAllocation(party, settlementID, description string) domain.Action {
	return domain.NewAction(domain.Intent{
		PartyID: party, Purpose: domain.PurposeAllocate, Ref: settlementID,
		Description: description,
	}, prepared("h"), time.Now(), 10*time.Minute)
}

func Test_Allocate_SpendsAHoldingWhoseLockLapsed(t *testing.T) {
	d := newDeps(t)
	u := d.startedWithFaucet(t, faucetConfig(t))

	lapsed := holding("h-lapsed", faucet, "CBTC", "1.0")
	lapsed.Locked = true
	lapsed.LockedUntil = time.Now().Add(-time.Hour)

	live := holding("h-live", faucet, "CBTC", "5.0")
	live.Locked = true
	live.LockedUntil = time.Now().Add(time.Hour)

	d.canton.EXPECT().Holdings(Any, faucet).Return([]domain.Holding{
		lapsed, live, holding("h-usdcx", faucet, "USDCx", "1000000.0"),
	}, nil).Once()
	d.registry.EXPECT().AllocateCommand(Any, allocateSpec("CBTC", "h-lapsed")).Return(command("cbtc"), nil).Once()
	d.registry.EXPECT().AllocateCommand(Any, allocateSpec("USDCx", "h-usdcx")).Return(command("usdcx"), nil).Once()
	d.canton.EXPECT().Submit(Any, faucet, Any, Any).Return(domain.Transaction{}, nil).Once()
	d.events.EXPECT().Publish(Any, eventOfType(events.WalletUpdated), parties(faucet)).Return(nil).Once()

	output, err := u.Allocate(t.Context(), dto.AllocateInput{
		SettlementID: "s-1",
		Deadline:     time.Now().Add(time.Hour),
		Legs:         tradeLegs(faucet, alice),
		PartyID:      faucet,
	})
	require.NoError(t, err)
	require.True(t, output.Executed)
}

func Test_Allocate_InsufficientFunds(t *testing.T) {
	d := newDeps(t)
	u := d.startedWithFaucet(t, faucetConfig(t))

	d.canton.EXPECT().Holdings(Any, faucet).Return([]domain.Holding{
		holding("h-cbtc", faucet, "CBTC", "0.5"),
		holding("h-usdcx", faucet, "USDCx", "1000000.0"),
	}, nil).Once()

	_, err := u.Allocate(t.Context(), dto.AllocateInput{
		SettlementID: "s-1",
		Deadline:     time.Now().Add(time.Hour),
		Legs:         tradeLegs(faucet, alice),
		PartyID:      faucet,
	})
	require.ErrorIs(t, err, domain.ErrInsufficientFunds)
	require.ErrorContains(t, err, "insufficient funds: CBTC: need 1.0, held 0.5")
}

func Test_Allocate_Validation(t *testing.T) {
	d := newDeps(t)
	u := d.started(t)

	for name, input := range map[string]dto.AllocateInput{
		"bad amount": {
			SettlementID: "s-1", Deadline: time.Now().Add(time.Hour), PartyID: maker,
			Legs: []dto.TransferLeg{{ID: "base", Sender: maker, Receiver: alice, Instrument: "CBTC", Amount: "1e5"}},
		},
		"deadline passed": {
			SettlementID: "s-1", Deadline: time.Now().Add(-time.Minute), PartyID: maker,
			Legs: tradeLegs(maker, alice),
		},
		"not a party of the legs": {
			SettlementID: "s-1", Deadline: time.Now().Add(time.Hour), PartyID: bob,
			Legs: tradeLegs(maker, alice),
		},
	} {
		_, err := u.Allocate(t.Context(), input)
		require.ErrorIs(t, err, domain.ErrValidation, name)
	}

	_, err := u.Allocate(t.Context(), dto.AllocateInput{
		SettlementID: "s-1", Deadline: time.Now().Add(time.Hour), PartyID: maker,
		Legs: []dto.TransferLeg{{ID: "base", Sender: maker, Receiver: alice, Instrument: "DOGE", Amount: "1"}},
	})
	require.ErrorIs(t, err, domain.ErrUnknownInstrument)
}
