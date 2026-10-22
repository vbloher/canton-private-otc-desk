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
	d.cachedRules()
	d.canton.EXPECT().TransferCommand(rules, mock.MatchedBy(func(s domain.TransferSpec) bool {
		return s.Admin == operator && s.Sender == alice && s.Receiver == makerB && s.InstrumentID == "CBTC" &&
			s.Amount.String() == "1.5" && len(s.Inputs) == 1 && s.Inputs[0] == "h-1" &&
			s.ExecuteBefore.After(time.Now().Add(23*time.Hour))
	})).Return(command("transfer"), nil).Once()
	d.canton.EXPECT().Prepare(Any, alice, mock.MatchedBy(func(id string) bool {
		return strings.HasPrefix(id, "withdraw:") // every withdrawal is a new intent
	}), command("transfer")).Return(prepared("hash-t"), nil).Once()
	d.postgres.EXPECT().SaveActions(Any, mock.MatchedBy(func(actions []domain.Action) bool {
		return len(actions) == 1 && actions[0].Purpose == domain.PurposeTransferOut &&
			actions[0].Description == "Withdraw 1.5 CBTC to makerB" &&
			actions[0].Ref == makerB && actions[0].Counterparty == makerB
	})).Return(nil).Once()
	d.events.EXPECT().Publish(Any, eventOfType(events.SignRequested), parties(alice)).Return(nil).Once()

	output, err := u.Withdraw(t.Context(), dto.WithdrawInput{PartyID: alice, Symbol: "CBTC", Amount: "1.5", Receiver: makerB})
	require.NoError(t, err)
	require.Len(t, output.Actions, 1)
}

func Test_Withdraw_SamePendingWithdrawalIsReturned(t *testing.T) {
	d := newDeps(t)
	u := d.started(t)

	queued := domain.NewAction(domain.Intent{PartyID: alice, Purpose: domain.PurposeTransferOut, Ref: makerB,
		Counterparty: makerB, Description: "Withdraw 1.5 CBTC to makerB"}, prepared("hash-t"), time.Now(), time.Minute)
	otherAmount := domain.NewAction(domain.Intent{PartyID: alice, Purpose: domain.PurposeTransferOut, Ref: makerB,
		Counterparty: makerB, Description: "Withdraw 9.0 CBTC to makerB"}, prepared("hash-9"), time.Now(), time.Minute)

	// A double-submitted withdrawal: the same one is still waiting for the signature.
	d.postgres.EXPECT().ListPendingActions(Any, alice, Any).Return([]domain.Action{otherAmount, queued}, nil).Once()

	output, err := u.Withdraw(t.Context(), dto.WithdrawInput{PartyID: alice, Symbol: "CBTC", Amount: "1.5", Receiver: makerB})
	require.NoError(t, err)
	require.Equal(t, []domain.Action{queued}, output.Actions)
}

func Test_Withdraw_HostedPartyNotifiesTheReceiver(t *testing.T) {
	d := newDeps(t)
	u := d.started(t)

	d.canton.EXPECT().Holdings(Any, makerA).Return([]domain.Holding{holding("h-1", makerA, "CBTC", "2.0")}, nil).Once()
	d.cachedRules()
	d.canton.EXPECT().TransferCommand(rules, Any).Return(command("transfer"), nil).Once()
	d.canton.EXPECT().Submit(Any, makerA, mock.AnythingOfType("string"), command("transfer")).
		Return(domain.Transaction{}, nil).Once()
	d.events.EXPECT().Publish(Any, eventOfType(events.WalletUpdated), parties(makerA, alice)).Return(nil).Once()

	output, err := u.Withdraw(t.Context(), dto.WithdrawInput{PartyID: makerA, Symbol: "CBTC", Amount: "1", Receiver: alice})
	require.NoError(t, err)
	require.Empty(t, output.Actions)
}

func Test_Withdraw_Validation(t *testing.T) {
	d := newDeps(t)
	u := d.started(t)

	for reason, input := range map[string]dto.WithdrawInput{
		`receiver "makerB" must be a full party id`: {PartyID: alice, Symbol: "CBTC", Amount: "1", Receiver: "makerB"},
		"cannot withdraw to yourself":               {PartyID: alice, Symbol: "CBTC", Amount: "1", Receiver: alice},
		`amount "0" is not a positive decimal`:      {PartyID: alice, Symbol: "CBTC", Amount: "0", Receiver: makerB},
	} {
		_, err := u.Withdraw(t.Context(), input)
		require.ErrorIs(t, err, domain.ErrValidation, reason)
		require.ErrorContains(t, err, reason)
	}

	_, err := u.Withdraw(t.Context(), dto.WithdrawInput{PartyID: alice, Symbol: "DOGE", Amount: "1", Receiver: makerB})
	require.ErrorIs(t, err, domain.ErrUnknownInstrument)
}

func Test_Withdraw_InsufficientFunds(t *testing.T) {
	d := newDeps(t)
	u := d.started(t)

	d.postgres.EXPECT().ListPendingActions(Any, alice, Any).Return(nil, nil).Once()
	d.canton.EXPECT().Holdings(Any, alice).Return([]domain.Holding{holding("h-1", alice, "CBTC", "1.0")}, nil).Once()

	_, err := u.Withdraw(t.Context(), dto.WithdrawInput{PartyID: alice, Symbol: "CBTC", Amount: "1.5", Receiver: makerB})
	require.ErrorIs(t, err, domain.ErrInsufficientFunds)
}

func Test_AcceptIncoming_UnknownTransfer(t *testing.T) {
	d := newDeps(t)
	u := d.started(t)

	d.postgres.EXPECT().ListPendingActions(Any, alice, Any).Return(nil, nil).Once()
	d.canton.EXPECT().TransferInstructions(Any, alice).Return([]domain.IncomingTransfer{
		{ContractID: "other", Sender: makerA, Admin: operator, InstrumentID: "CBTC", Amount: decimal.MustParse("1")},
		// Another admin's instrument is not the desk's: its offer is not acceptable here.
		{ContractID: "offer-1", Sender: makerA, Admin: "someone-else::1220", InstrumentID: "CBTC", Amount: decimal.MustParse("1")},
	}, nil).Once()

	_, err := u.AcceptIncoming(t.Context(), dto.AcceptIncomingInput{PartyID: alice, ContractID: "offer-1"})
	require.ErrorIs(t, err, domain.ErrNotFound)
}

func Test_AcceptIncoming_ExternalPartyEnqueues(t *testing.T) {
	d := newDeps(t)
	u := d.started(t)

	d.postgres.EXPECT().ListPendingActions(Any, alice, Any).Return(nil, nil).Once()
	d.canton.EXPECT().TransferInstructions(Any, alice).Return([]domain.IncomingTransfer{
		{ContractID: "offer-1", Sender: makerA, Admin: operator, InstrumentID: "CBTC", Amount: decimal.MustParse("2.5")},
	}, nil).Once()
	d.cachedRules()
	d.canton.EXPECT().AcceptTransferCommand(rules, noInputs, "offer-1").Return(command("accept"), nil).Once()
	d.canton.EXPECT().Prepare(Any, alice, "accept:offer-1", command("accept")).Return(prepared("hash-a"), nil).Once()
	d.postgres.EXPECT().SaveActions(Any, mock.MatchedBy(func(actions []domain.Action) bool {
		return len(actions) == 1 && actions[0].Ref == "offer-1" && actions[0].Description == "Accept 2.5 CBTC from makerA" &&
			actions[0].Counterparty == makerA
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

	d.canton.EXPECT().Holdings(Any, alice).Return([]domain.Holding{holding("h-1", alice, "CETH", "3.0")}, nil).Once()

	output, err := u.GetBalances(t.Context(), dto.GetBalancesInput{PartyID: alice})
	require.NoError(t, err)
	require.Len(t, output.Balances, 3)
	require.Equal(t, "CETH", output.Balances[2].Symbol)
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

func Test_AcceptIncoming_DisclosesLockedInputs(t *testing.T) {
	d := newDeps(t)
	u := d.started(t)

	// A withdrawal's offer consumes the sender's locked holding, which the receiver cannot
	// see: the operator, its admin, discloses it.
	d.canton.EXPECT().TransferInstructions(Any, makerB).Return([]domain.IncomingTransfer{
		{ContractID: "offer-1", Sender: alice, Admin: operator, InstrumentID: "CBTC", Amount: decimal.MustParse("0.4"),
			Inputs: []string{"locked-1"}},
	}, nil).Once()
	d.cachedRules()

	locked := domain.Disclosed{TemplateID: "pkg:Token", ContractID: "locked-1", CreatedEventBlob: "blob-locked"}

	d.canton.EXPECT().Tokens(Any, operator).Return([]domain.Disclosed{
		{TemplateID: "pkg:Token", ContractID: "unrelated", CreatedEventBlob: "blob-other"},
		locked,
	}, nil).Once()
	d.canton.EXPECT().AcceptTransferCommand(rules, []domain.Disclosed{locked}, "offer-1").Return(command("accept"), nil).Once()
	d.canton.EXPECT().Submit(Any, makerB, "accept:offer-1", command("accept")).Return(domain.Transaction{}, nil).Once()
	// The sender's locked holding is gone now: its wallet changed too.
	d.events.EXPECT().Publish(Any, eventOfType(events.WalletUpdated), parties(makerB, alice)).Return(nil).Once()

	output, err := u.AcceptIncoming(t.Context(), dto.AcceptIncomingInput{PartyID: makerB, ContractID: "offer-1"})
	require.NoError(t, err)
	require.Empty(t, output.Actions)
}

func Test_AcceptIncoming_PendingAcceptIsReturned(t *testing.T) {
	d := newDeps(t)
	u := d.started(t)

	queued := domain.NewAction(domain.Intent{PartyID: alice, Purpose: domain.PurposeTransferAccept, Ref: "offer-1",
		Counterparty: makerA, Description: "Accept 2.5 CBTC from makerA"}, prepared("hash-a"), time.Now(), time.Minute)

	d.postgres.EXPECT().ListPendingActions(Any, alice, Any).Return([]domain.Action{queued}, nil).Once()

	output, err := u.AcceptIncoming(t.Context(), dto.AcceptIncomingInput{PartyID: alice, ContractID: "offer-1"})
	require.NoError(t, err)
	require.Equal(t, []domain.Action{queued}, output.Actions)
}

func Test_ListIncoming_OnlyTheDesksInstruments(t *testing.T) {
	d := newDeps(t)
	u := d.started(t)

	mint := domain.IncomingTransfer{ContractID: "mint-1", Admin: operator, InstrumentID: "CBTC", Amount: decimal.MustParse("1")}
	foreign := domain.IncomingTransfer{ContractID: "x-1", Sender: makerA, Admin: "someone-else::1220", InstrumentID: "CBTC",
		Amount: decimal.MustParse("1")}

	d.canton.EXPECT().TransferInstructions(Any, alice).Return([]domain.IncomingTransfer{mint, foreign}, nil).Once()

	output, err := u.ListIncoming(t.Context(), dto.ListIncomingInput{PartyID: alice})
	require.NoError(t, err)
	require.Equal(t, []domain.IncomingTransfer{mint}, output.Transfers)
}
