package usecase_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/pkg/decimal"
	"github.com/vbloher/canton-private-otc-desk/pkg/events"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/dto"
)

func Test_Withdraw_ExternalPartyEnqueues(t *testing.T) {
	d := newDeps(t)
	u := d.started(t)

	d.postgres.EXPECT().ListPendingActions(Any, alice, Any).Return(nil, nil).Once()
	d.canton.EXPECT().Holdings(Any, alice).Return([]domain.Holding{holding("h-1", alice, "CBTC", "2.0")}, nil).Once()
	d.registry.EXPECT().TransferCommand(Any, mock.MatchedBy(func(s domain.TransferSpec) bool {
		return s.Admin == operator && s.Sender == alice && s.Receiver == bob && s.InstrumentID == "CBTC" &&
			s.Amount.String() == "1.5" && len(s.Inputs) == 1 && s.Inputs[0] == "h-1" &&
			s.ExecuteBefore.After(time.Now().Add(23*time.Hour))
	})).Return(command("transfer"), nil).Once()
	d.canton.EXPECT().Prepare(Any, alice, mock.MatchedBy(func(id string) bool {
		return strings.HasPrefix(id, "withdraw:")
	}), command("transfer")).Return(prepared("hash-t"), nil).Once()
	d.postgres.EXPECT().SaveActions(Any, mock.MatchedBy(func(actions []domain.Action) bool {
		return len(actions) == 1 && actions[0].Purpose == domain.PurposeTransferOut &&
			actions[0].Description == "Withdraw 1.5 CBTC to bob" &&
			actions[0].Ref == bob && actions[0].Counterparty == bob
	})).Return(nil).Once()
	d.events.EXPECT().Publish(Any, eventOfType(events.SignRequested), parties(alice)).Return(nil).Once()

	output, err := u.Withdraw(t.Context(), dto.WithdrawInput{PartyID: alice, Symbol: "CBTC", Amount: "1.5", Receiver: bob})
	require.NoError(t, err)
	require.Len(t, output.Actions, 1)
}

func Test_Withdraw_SamePendingWithdrawalIsReturned(t *testing.T) {
	d := newDeps(t)
	u := d.started(t)

	queued := domain.NewAction(domain.Intent{
		PartyID: alice, Purpose: domain.PurposeTransferOut, Ref: bob,
		Counterparty: bob, Description: "Withdraw 1.5 CBTC to bob",
	}, prepared("hash-t"), time.Now(), time.Minute)
	otherAmount := domain.NewAction(domain.Intent{
		PartyID: alice, Purpose: domain.PurposeTransferOut, Ref: bob,
		Counterparty: bob, Description: "Withdraw 9.0 CBTC to bob",
	}, prepared("hash-9"), time.Now(), time.Minute)

	d.postgres.EXPECT().ListPendingActions(Any, alice, Any).Return([]domain.Action{otherAmount, queued}, nil).Once()

	output, err := u.Withdraw(t.Context(), dto.WithdrawInput{PartyID: alice, Symbol: "CBTC", Amount: "1.5", Receiver: bob})
	require.NoError(t, err)
	require.Equal(t, []domain.Action{queued}, output.Actions)
}

func Test_Withdraw_HostedPartyNotifiesTheReceiver(t *testing.T) {
	d := newDeps(t)
	u := d.started(t)

	d.canton.EXPECT().Holdings(Any, maker).Return([]domain.Holding{holding("h-1", maker, "CBTC", "2.0")}, nil).Once()
	d.registry.EXPECT().TransferCommand(Any, Any).Return(command("transfer"), nil).Once()
	d.canton.EXPECT().Submit(Any, maker, mock.AnythingOfType("string"), command("transfer")).
		Return(domain.Transaction{}, nil).Once()
	d.events.EXPECT().Publish(Any, eventOfType(events.WalletUpdated), parties(maker, alice)).Return(nil).Once()

	output, err := u.Withdraw(t.Context(), dto.WithdrawInput{PartyID: maker, Symbol: "CBTC", Amount: "1", Receiver: alice})
	require.NoError(t, err)
	require.Empty(t, output.Actions)
}

func Test_Withdraw_Validation(t *testing.T) {
	d := newDeps(t)
	u := d.started(t)

	for reason, input := range map[string]dto.WithdrawInput{
		`receiver "bob" must be a full party id`: {PartyID: alice, Symbol: "CBTC", Amount: "1", Receiver: "bob"},
		"cannot withdraw to yourself":            {PartyID: alice, Symbol: "CBTC", Amount: "1", Receiver: alice},
		`amount "0" is not a positive decimal`:   {PartyID: alice, Symbol: "CBTC", Amount: "0", Receiver: bob},
	} {
		_, err := u.Withdraw(t.Context(), input)
		require.ErrorIs(t, err, domain.ErrValidation, reason)
		require.ErrorContains(t, err, reason)
	}

	_, err := u.Withdraw(t.Context(), dto.WithdrawInput{PartyID: alice, Symbol: "DOGE", Amount: "1", Receiver: bob})
	require.ErrorIs(t, err, domain.ErrUnknownInstrument)
}

func Test_Withdraw_InsufficientFunds(t *testing.T) {
	d := newDeps(t)
	u := d.started(t)

	d.postgres.EXPECT().ListPendingActions(Any, alice, Any).Return(nil, nil).Once()
	d.canton.EXPECT().Holdings(Any, alice).Return([]domain.Holding{holding("h-1", alice, "CBTC", "1.0")}, nil).Once()

	_, err := u.Withdraw(t.Context(), dto.WithdrawInput{PartyID: alice, Symbol: "CBTC", Amount: "1.5", Receiver: bob})
	require.ErrorIs(t, err, domain.ErrInsufficientFunds)
}

func Test_AcceptIncoming_UnknownTransfer(t *testing.T) {
	d := newDeps(t)
	u := d.started(t)

	d.postgres.EXPECT().ListPendingActions(Any, alice, Any).Return(nil, nil).Once()
	d.canton.EXPECT().TransferInstructions(Any, alice).Return([]domain.IncomingTransfer{
		{ContractID: "other", Sender: maker, Admin: operator, InstrumentID: "CBTC", Amount: decimal.MustParse("1")},
		{ContractID: "offer-1", Sender: maker, Admin: "someone-else::1220", InstrumentID: "CBTC", Amount: decimal.MustParse("1")},
	}, nil).Once()

	_, err := u.AcceptIncoming(t.Context(), dto.AcceptIncomingInput{PartyID: alice, ContractID: "offer-1"})
	require.ErrorIs(t, err, domain.ErrNotFound)
}

func Test_AcceptIncoming_ExternalPartyEnqueues(t *testing.T) {
	d := newDeps(t)
	u := d.started(t)

	d.postgres.EXPECT().ListPendingActions(Any, alice, Any).Return(nil, nil).Once()
	d.canton.EXPECT().TransferInstructions(Any, alice).Return([]domain.IncomingTransfer{
		{ContractID: "offer-1", Sender: maker, Admin: operator, InstrumentID: "CBTC", Amount: decimal.MustParse("2.5")},
	}, nil).Once()
	d.registry.EXPECT().AcceptTransferCommand(Any, registryURL, "offer-1").Return(command("accept"), nil).Once()
	d.canton.EXPECT().Prepare(Any, alice, "accept:offer-1", command("accept")).Return(prepared("hash-a"), nil).Once()
	d.postgres.EXPECT().SaveActions(Any, mock.MatchedBy(func(actions []domain.Action) bool {
		return len(actions) == 1 && actions[0].Ref == "offer-1" && actions[0].Description == "Accept 2.5 CBTC from maker" &&
			actions[0].Counterparty == maker
	})).Return(nil).Once()
	d.events.EXPECT().Publish(Any, eventOfType(events.SignRequested), parties(alice)).Return(nil).Once()

	output, err := u.AcceptIncoming(t.Context(), dto.AcceptIncomingInput{PartyID: alice, ContractID: "offer-1"})
	require.NoError(t, err)
	require.Len(t, output.Actions, 1)
}

func Test_GetBalances_LapsedLockIsAvailable(t *testing.T) {
	d := newDeps(t)
	u := d.started(t)

	lapsed := holding("h-lapsed", alice, "CBTC", "0.4")
	lapsed.Locked = true
	lapsed.LockedUntil = time.Now().Add(-time.Hour)

	forever := holding("h-forever", alice, "CBTC", "0.1")
	forever.Locked = true

	d.canton.EXPECT().Holdings(Any, alice).Return([]domain.Holding{lapsed, forever}, nil).Once()

	output, err := u.GetBalances(t.Context(), dto.GetBalancesInput{PartyID: alice})
	require.NoError(t, err)
	require.Equal(t, "0.4", output.Balances[0].Available.String())
	require.Equal(t, "0.1", output.Balances[0].Locked.String())
}

func Test_GetBalances_EveryInstrument(t *testing.T) {
	d := newDeps(t)
	u := d.started(t)

	amulet := holding("h-1", alice, "Amulet", "3.0")
	amulet.Admin = dso
	wrongAdmin := holding("h-2", alice, "Amulet", "9.0")

	d.canton.EXPECT().Holdings(Any, alice).Return([]domain.Holding{amulet, wrongAdmin}, nil).Once()

	output, err := u.GetBalances(t.Context(), dto.GetBalancesInput{PartyID: alice})
	require.NoError(t, err)
	require.Len(t, output.Balances, 3)
	require.Equal(t, "CC", output.Balances[2].Symbol)
	require.Equal(t, "3.0", output.Balances[2].Available.String())
}

func Test_GenerateExternalParty_UsesCachedSynchronizer(t *testing.T) {
	d := newDeps(t)
	u := d.started(t)

	key := make([]byte, 32)
	topology := domain.Topology{PartyID: "alice-3f9a1c::1220bb", Fingerprint: "1220bb", MultiHash: []byte("mh")}

	d.redis.EXPECT().GetSynchronizerID(Any).Return(syncID, true).Once()
	d.canton.EXPECT().GenerateExternalParty(Any, syncID, "alice-3f9a1c", key).Return(topology, nil).Once()

	output, err := u.GenerateExternalParty(t.Context(), dto.GenerateExternalPartyInput{Hint: "alice-3f9a1c", PublicKey: key})
	require.NoError(t, err)
	require.Equal(t, topology, output.Topology)

	_, err = u.GenerateExternalParty(t.Context(), dto.GenerateExternalPartyInput{Hint: "alice", PublicKey: key[:31]})
	require.ErrorIs(t, err, domain.ErrValidation)
}

func Test_AcceptIncoming_UsesTheInstrumentsRegistry(t *testing.T) {
	d := newDeps(t)
	u := d.started(t)

	d.canton.EXPECT().TransferInstructions(Any, maker).Return([]domain.IncomingTransfer{
		{ContractID: "offer-1", Sender: alice, Admin: dso, InstrumentID: "Amulet", Amount: decimal.MustParse("40")},
	}, nil).Once()
	d.registry.EXPECT().AcceptTransferCommand(Any, scanURL, "offer-1").Return(command("accept"), nil).Once()
	d.canton.EXPECT().Submit(Any, maker, "accept:offer-1", command("accept")).Return(domain.Transaction{}, nil).Once()
	d.events.EXPECT().Publish(Any, eventOfType(events.WalletUpdated), parties(maker, alice)).Return(nil).Once()

	output, err := u.AcceptIncoming(t.Context(), dto.AcceptIncomingInput{PartyID: maker, ContractID: "offer-1"})
	require.NoError(t, err)
	require.Empty(t, output.Actions)
}

func Test_AcceptIncoming_PendingAcceptIsReturned(t *testing.T) {
	d := newDeps(t)
	u := d.started(t)

	queued := domain.NewAction(domain.Intent{
		PartyID: alice, Purpose: domain.PurposeTransferAccept, Ref: "offer-1",
		Counterparty: maker, Description: "Accept 2.5 CBTC from maker",
	}, prepared("hash-a"), time.Now(), time.Minute)

	d.postgres.EXPECT().ListPendingActions(Any, alice, Any).Return([]domain.Action{queued}, nil).Once()

	output, err := u.AcceptIncoming(t.Context(), dto.AcceptIncomingInput{PartyID: alice, ContractID: "offer-1"})
	require.NoError(t, err)
	require.Equal(t, []domain.Action{queued}, output.Actions)
}

func Test_ListIncoming_OnlyTheDesksInstruments(t *testing.T) {
	d := newDeps(t)
	u := d.started(t)

	mint := domain.IncomingTransfer{ContractID: "mint-1", Admin: operator, InstrumentID: "CBTC", Amount: decimal.MustParse("1")}
	foreign := domain.IncomingTransfer{
		ContractID: "x-1", Sender: maker, Admin: "someone-else::1220", InstrumentID: "CBTC",
		Amount: decimal.MustParse("1"),
	}

	d.canton.EXPECT().TransferInstructions(Any, alice).Return([]domain.IncomingTransfer{mint, foreign}, nil).Once()

	output, err := u.ListIncoming(t.Context(), dto.ListIncomingInput{PartyID: alice})
	require.NoError(t, err)

	mint.Symbol = "CBTC"
	require.Equal(t, []domain.IncomingTransfer{mint}, output.Transfers)
}
