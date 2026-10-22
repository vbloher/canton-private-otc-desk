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
	u := d.started(t)

	d.canton.EXPECT().Holdings(Any, makerA).Return([]domain.Holding{
		holding("h-cbtc", makerA, "CBTC", "10.0"),
		holding("h-usdcx", makerA, "USDCx", "1000000.0"),
	}, nil).Once()
	d.cachedRules()
	d.canton.EXPECT().AllocateCommand(rules, allocateSpec("CBTC", "h-cbtc")).Return(command("cbtc"), nil).Once()
	d.canton.EXPECT().AllocateCommand(rules, allocateSpec("USDCx", "h-usdcx")).Return(command("usdcx"), nil).Once()
	d.canton.EXPECT().Submit(Any, makerA, "allocate:s-1:"+makerA+":CBTC", command("cbtc")).
		Return(domain.Transaction{UpdateID: "u-1"}, nil).Once()
	d.canton.EXPECT().Submit(Any, makerA, "allocate:s-1:"+makerA+":USDCx", command("usdcx")).
		Return(domain.Transaction{UpdateID: "u-2"}, nil).Once()
	d.events.EXPECT().Publish(Any, event(events.WalletUpdated, `{}`), parties(makerA)).Return(nil).Once()

	output, err := u.Allocate(t.Context(), dto.AllocateInput{
		SettlementID: "s-1",
		Deadline:     time.Now().Add(time.Hour),
		Legs:         tradeLegs(makerA, alice),
		PartyID:      makerA,
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
	d.cachedRules()
	// alice receives CBTC (funds nothing) and sends the 100000 USDCx notional.
	d.canton.EXPECT().AllocateCommand(rules, allocateSpec("CBTC")).Return(command("cbtc"), nil).Once()
	d.canton.EXPECT().AllocateCommand(rules, allocateSpec("USDCx", "h-usdcx")).Return(command("usdcx"), nil).Once()
	// The deterministic command id travels with the prepared transaction: the ledger executes
	// one allocation per id, however many times the party signs.
	d.canton.EXPECT().Prepare(Any, alice, "allocate:s-1:"+alice+":CBTC", command("cbtc")).
		Return(prepared("hash-cbtc"), nil).Once()
	d.canton.EXPECT().Prepare(Any, alice, "allocate:s-1:"+alice+":USDCx", command("usdcx")).
		Return(prepared("hash-usdcx"), nil).Once()

	var saved []domain.Action

	d.postgres.EXPECT().SaveActions(Any, mock.MatchedBy(func(actions []domain.Action) bool {
		saved = actions

		return len(actions) == 2
	})).Return(nil).Once()
	d.events.EXPECT().Publish(Any, eventOfType(events.SignRequested), parties(alice)).Return(nil).Times(2)

	output, err := u.Allocate(t.Context(), dto.AllocateInput{
		SettlementID: "s-1",
		Deadline:     time.Now().Add(time.Hour),
		Legs:         tradeLegs(makerA, alice),
		PartyID:      alice,
		Description:  "quote s-1",
	})
	require.NoError(t, err)
	require.False(t, output.Executed)

	ids := make([]uuid.UUID, 0, len(saved))

	for i, a := range saved {
		ids = append(ids, a.ID)

		require.Equal(t, domain.ActionPending, a.Status)
		require.Equal(t, alice, a.PartyID)
		require.Equal(t, domain.PurposeAllocate, a.Purpose)
		require.Equal(t, "s-1", a.Ref)
		require.Equal(t, []string{"hash-cbtc", "hash-usdcx"}[i], string(a.Hash))
		require.Equal(t, a.Hash, a.Prepared.Hash)
		require.WithinDuration(t, time.Now().Add(10*time.Minute), a.ExpiresAt, time.Minute)
	}

	require.Contains(t, saved[0].Description, "CBTC")
	require.Contains(t, saved[1].Description, "100000.0 USDCx")
	require.Equal(t, ids, output.ActionIDs)
}

func Test_Allocate_ExternalPartyGetsItsPendingActionsBack(t *testing.T) {
	d := newDeps(t)
	u := d.started(t)

	queued := []domain.Action{
		pendingAllocation(alice, "s-1", "Allocate receipt of CBTC for quote s-1"),
		pendingAllocation(alice, "s-1", "Allocate 100000.0 USDCx for quote s-1"),
	}
	other := pendingAllocation(alice, "s-2", "Allocate 1.0 CBTC for quote s-2")

	// A retried Allocate: the party has not signed yet, so nothing is prepared or queued again.
	d.postgres.EXPECT().ListPendingActions(Any, alice, Any).Return(append(queued, other), nil).Once()

	output, err := u.Allocate(t.Context(), dto.AllocateInput{
		SettlementID: "s-1",
		Deadline:     time.Now().Add(time.Hour),
		Legs:         tradeLegs(makerA, alice),
		PartyID:      alice,
	})
	require.NoError(t, err)
	require.False(t, output.Executed)
	require.Equal(t, []uuid.UUID{queued[0].ID, queued[1].ID}, output.ActionIDs)
}

func pendingAllocation(party, settlementID, description string) domain.Action {
	return domain.NewAction(domain.Intent{PartyID: party, Purpose: domain.PurposeAllocate, Ref: settlementID,
		Description: description}, prepared("h"), time.Now(), 10*time.Minute)
}

func Test_Allocate_SpendsAHoldingWhoseLockLapsed(t *testing.T) {
	d := newDeps(t)
	u := d.started(t)

	// The CBTC is still marked locked by an allocation whose settlement deadline has passed
	// (its release failed): the ledger accepts it as an unlocked input again.
	lapsed := holding("h-lapsed", makerA, "CBTC", "1.0")
	lapsed.Locked = true
	lapsed.LockedUntil = time.Now().Add(-time.Hour)

	live := holding("h-live", makerA, "CBTC", "5.0")
	live.Locked = true
	live.LockedUntil = time.Now().Add(time.Hour)

	d.canton.EXPECT().Holdings(Any, makerA).Return([]domain.Holding{
		lapsed, live, holding("h-usdcx", makerA, "USDCx", "1000000.0"),
	}, nil).Once()
	d.cachedRules()
	d.canton.EXPECT().AllocateCommand(rules, allocateSpec("CBTC", "h-lapsed")).Return(command("cbtc"), nil).Once()
	d.canton.EXPECT().AllocateCommand(rules, allocateSpec("USDCx", "h-usdcx")).Return(command("usdcx"), nil).Once()
	d.canton.EXPECT().Submit(Any, makerA, Any, Any).Return(domain.Transaction{}, nil).Times(2)
	d.events.EXPECT().Publish(Any, eventOfType(events.WalletUpdated), parties(makerA)).Return(nil).Once()

	output, err := u.Allocate(t.Context(), dto.AllocateInput{
		SettlementID: "s-1",
		Deadline:     time.Now().Add(time.Hour),
		Legs:         tradeLegs(makerA, alice),
		PartyID:      makerA,
	})
	require.NoError(t, err)
	require.True(t, output.Executed)
}

func Test_Allocate_InsufficientFunds(t *testing.T) {
	d := newDeps(t)
	u := d.started(t)

	d.canton.EXPECT().Holdings(Any, makerA).Return([]domain.Holding{
		holding("h-cbtc", makerA, "CBTC", "0.5"),
		holding("h-usdcx", makerA, "USDCx", "1000000.0"),
	}, nil).Once()

	_, err := u.Allocate(t.Context(), dto.AllocateInput{
		SettlementID: "s-1",
		Deadline:     time.Now().Add(time.Hour),
		Legs:         tradeLegs(makerA, alice),
		PartyID:      makerA,
	})
	require.ErrorIs(t, err, domain.ErrInsufficientFunds)
	require.ErrorContains(t, err, "insufficient funds: CBTC: need 1.0, held 0.5")
}

func Test_Allocate_Validation(t *testing.T) {
	d := newDeps(t)
	u := d.started(t)

	for name, input := range map[string]dto.AllocateInput{
		"bad amount": {SettlementID: "s-1", Deadline: time.Now().Add(time.Hour), PartyID: makerA,
			Legs: []dto.TransferLeg{{ID: "base", Sender: makerA, Receiver: alice, Instrument: "CBTC", Amount: "1e5"}}},
		"deadline passed": {SettlementID: "s-1", Deadline: time.Now().Add(-time.Minute), PartyID: makerA,
			Legs: tradeLegs(makerA, alice)},
		"not a party of the legs": {SettlementID: "s-1", Deadline: time.Now().Add(time.Hour), PartyID: makerB,
			Legs: tradeLegs(makerA, alice)},
	} {
		_, err := u.Allocate(t.Context(), input)
		require.ErrorIs(t, err, domain.ErrValidation, name)
	}

	_, err := u.Allocate(t.Context(), dto.AllocateInput{
		SettlementID: "s-1", Deadline: time.Now().Add(time.Hour), PartyID: makerA,
		Legs: []dto.TransferLeg{{ID: "base", Sender: makerA, Receiver: alice, Instrument: "DOGE", Amount: "1"}},
	})
	require.ErrorIs(t, err, domain.ErrUnknownInstrument)
}
