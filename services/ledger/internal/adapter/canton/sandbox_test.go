//go:build sandbox

// Live tests against the local Canton sandbox (make ledger). They create fresh external
// parties and never touch the seeded makers' balances.
package canton_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/pkg/decimal"
	"github.com/vbloher/canton-private-otc-desk/pkg/otel"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/adapter/canton"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/domain"
)

const (
	eventually = 30 * time.Second
	tick       = 250 * time.Millisecond
)

type sandbox struct {
	t        *testing.T
	ctx      context.Context
	ledger   *canton.Canton
	operator string
	sync     string
	rules    domain.Disclosed
}

func newSandbox(t *testing.T) *sandbox {
	t.Helper()

	otel.SilentModeInit()

	url := os.Getenv("CANTON_URL")
	if url == "" {
		url = "http://localhost:6864"
	}

	s := &sandbox{t: t, ctx: t.Context(), ledger: canton.New(canton.Config{URL: url, UserID: "participant_admin"})}

	require.True(t, s.ledger.Ready(s.ctx), "the sandbox is not running: make ledger")

	var err error

	s.operator, err = s.ledger.ResolveParty(s.ctx, "operator")
	require.NoError(t, err)

	s.sync, err = s.ledger.SynchronizerID(s.ctx, s.operator)
	require.NoError(t, err)

	s.rules, err = s.ledger.TokenRules(s.ctx, s.operator)
	require.NoError(t, err)

	return s
}

type externalParty struct {
	id  string
	key ed25519.PrivateKey
}

// onboard allocates an external party whose key lives here, as a browser would hold it.
func (s *sandbox) onboard(hint string) externalParty {
	s.t.Helper()

	public, private, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(s.t, err)

	topology, err := s.ledger.GenerateExternalParty(s.ctx, s.sync, hint+"-"+uuid.NewString()[:8], public)
	require.NoError(s.t, err)
	require.Equal(s.t, domain.Fingerprint(topology.PartyID), topology.Fingerprint)

	partyID, err := s.ledger.AllocateExternalParty(s.ctx, s.sync, topology.Transactions,
		ed25519.Sign(private, topology.MultiHash), topology.Fingerprint)
	require.NoError(s.t, err)
	require.Equal(s.t, topology.PartyID, partyID)

	return externalParty{id: partyID, key: private}
}

// sign is the browser's half of an interactive submission; Execute waits for the commit.
func (s *sandbox) sign(party externalParty, cmd domain.Command) {
	s.t.Helper()

	prepared, err := s.ledger.Prepare(s.ctx, party.id, "", cmd)
	require.NoError(s.t, err)
	require.NotEmpty(s.t, prepared.Hash)

	require.NoError(s.t, s.ledger.Execute(s.ctx, prepared, party.id, ed25519.Sign(party.key, prepared.Hash)))
}

func (s *sandbox) offerMint(receiver, instrument, amount string) string {
	s.t.Helper()

	cmd, err := s.ledger.OfferMintCommand(s.rules, domain.OfferMintSpec{
		Admin:        s.operator,
		Receiver:     receiver,
		InstrumentID: instrument,
		Amount:       decimal.MustParse(amount),
		OfferedAt:    time.Now().Add(-time.Minute),
	})
	require.NoError(s.t, err)

	tx, err := s.ledger.Submit(s.ctx, s.operator, "", cmd)
	require.NoError(s.t, err)
	require.NotEmpty(s.t, tx.UpdateID)
	require.Len(s.t, tx.Created, 1)

	return tx.Created[0]
}

// fund mints to the party and accepts the offer with its key.
func (s *sandbox) fund(party externalParty, instrument, amount string) {
	s.t.Helper()

	offer := s.offerMint(party.id, instrument, amount)

	accept, err := s.ledger.AcceptTransferCommand(s.rules, nil, offer)
	require.NoError(s.t, err)

	s.sign(party, accept)
	s.waitBalance(party.id, instrument, amount, "0.0")
}

// waitBalance waits for a submission's effects to reach the active contracts. It polls from
// another goroutine, so a failed read is reported with assert and polled again.
func (s *sandbox) waitBalance(party, instrument, available, locked string) {
	s.t.Helper()

	require.Eventually(s.t, func() bool {
		holdings, err := s.ledger.Holdings(s.ctx, party)
		if !assert.NoError(s.t, err) {
			return false
		}

		b := domain.Balances(holdings, party, s.operator, []domain.Instrument{{Symbol: instrument}}, time.Now())[0]

		return b.Available.String() == available && b.Locked.String() == locked
	}, eventually, tick, "%s %s: want %s available, %s locked", party, instrument, available, locked)
}

func Test_Sandbox_ExternalPartyLifecycle(t *testing.T) {
	s := newSandbox(t)
	alice := s.onboard("sandbox-alice")

	offer := s.offerMint(alice.id, "CBTC", "5.0")

	incoming, err := s.ledger.TransferInstructions(s.ctx, alice.id)
	require.NoError(t, err)
	require.Len(t, incoming, 1)
	require.Equal(t, offer, incoming[0].ContractID)
	require.Equal(t, "CBTC", incoming[0].InstrumentID)
	require.Equal(t, s.operator, incoming[0].Admin)
	require.Equal(t, "5.0", incoming[0].Amount.String())
	require.Empty(t, incoming[0].Inputs, "a mint offer locks no holding")
	require.Equal(t, domain.FaucetHint, incoming[0].SenderHint())

	accept, err := s.ledger.AcceptTransferCommand(s.rules, nil, offer)
	require.NoError(t, err)

	s.sign(alice, accept)

	s.waitBalance(alice.id, "CBTC", "5.0", "0.0")

	incoming, err = s.ledger.TransferInstructions(s.ctx, alice.id)
	require.NoError(t, err)
	require.Empty(t, incoming)
}

// allocate creates one slot's allocation: the operator submits its own, an external party signs.
func (s *sandbox) allocate(settlementID string, slot domain.Slot, parties map[string]externalParty) {
	s.t.Helper()

	var inputs []string

	if slot.Send.IsPositive() {
		holdings, err := s.ledger.Holdings(s.ctx, slot.Party)
		require.NoError(s.t, err)

		inputs, err = domain.SelectHoldings(holdings, slot.Party, slot.Admin, slot.InstrumentID, slot.Send, time.Now())
		require.NoError(s.t, err)
	}

	cmd, err := s.ledger.AllocateCommand(s.rules, domain.AllocateSpec{
		Operator:     s.operator,
		SettlementID: settlementID,
		Slot:         slot,
		Deadline:     time.Now().Add(time.Hour),
		RequestedAt:  time.Now().Add(-time.Minute),
		Inputs:       inputs,
	})
	require.NoError(s.t, err)

	if slot.Party == s.operator {
		_, err = s.ledger.Submit(s.ctx, s.operator, "allocate:"+settlementID+":"+slot.Party+":"+slot.InstrumentID, cmd)
		require.NoError(s.t, err)

		return
	}

	s.sign(parties[slot.Party], cmd)
}

func (s *sandbox) waitAllocated(settlementID string, plan []domain.Slot) []string {
	s.t.Helper()

	var cids []string

	require.Eventually(s.t, func() bool {
		allocations, err := s.ledger.Allocations(s.ctx, s.operator)
		if !assert.NoError(s.t, err) {
			return false
		}

		cids = cids[:0]

		for _, m := range domain.MatchSlots(allocations, settlementID, plan) {
			if m.ContractID == "" {
				return false
			}

			cids = append(cids, m.ContractID)
		}

		return true
	}, eventually, tick, "settlement %s: allocations missing", settlementID)

	return cids
}

func Test_Sandbox_SettleAndCancel(t *testing.T) {
	s := newSandbox(t)

	seller := s.onboard("sandbox-seller")
	buyer := s.onboard("sandbox-buyer")
	parties := map[string]externalParty{seller.id: seller, buyer.id: buyer}

	s.fund(seller, "CBTC", "1.0")
	s.fund(buyer, "USDCx", "100.0")

	// Settle: base seller → buyer, quote buyer → seller, fee buyer → operator.
	settlementID := uuid.NewString()
	legs := []domain.TransferLeg{
		{ID: "base", Sender: seller.id, Receiver: buyer.id, Instrument: "CBTC", Amount: decimal.MustParse("0.5")},
		{ID: "quote", Sender: buyer.id, Receiver: seller.id, Instrument: "USDCx", Amount: decimal.MustParse("50.0")},
		{ID: "fee", Sender: buyer.id, Receiver: s.operator, Instrument: "USDCx", Amount: decimal.MustParse("1.0")},
	}
	plan := domain.Plan(legs, s.operator)

	for _, slot := range plan {
		s.allocate(settlementID, slot, parties)
	}

	cids := s.waitAllocated(settlementID, plan)
	s.waitBalance(buyer.id, "USDCx", "49.0", "51.0")

	tokens, err := s.ledger.Tokens(s.ctx, s.operator)
	require.NoError(t, err)

	settle, err := s.ledger.SettleCommand(s.rules, tokens, domain.SettleSpec{
		Operator: s.operator, SettlementID: settlementID, Legs: legs, AllocationCIDs: cids,
	})
	require.NoError(t, err)

	tx, err := s.ledger.Submit(s.ctx, s.operator, "settle:"+settlementID, settle)
	require.NoError(t, err)
	require.NotEmpty(t, tx.UpdateID)

	// Once it has landed, a re-submitted settle fails interpretation before the command
	// dedup: its allocations are consumed.
	_, err = s.ledger.Submit(s.ctx, s.operator, "settle:"+settlementID, settle)
	require.ErrorIs(t, err, domain.ErrContractGone)
	require.ErrorIs(t, err, domain.ErrLedger)

	s.waitBalance(seller.id, "CBTC", "0.5", "0.0")
	s.waitBalance(seller.id, "USDCx", "50.0", "0.0")
	s.waitBalance(buyer.id, "CBTC", "0.5", "0.0")
	s.waitBalance(buyer.id, "USDCx", "49.0", "0.0")

	// Cancel: only the seller allocates a second settlement, the operator releases it.
	deadID := uuid.NewString()
	deadPlan := domain.Plan([]domain.TransferLeg{
		{ID: "base", Sender: seller.id, Receiver: buyer.id, Instrument: "CBTC", Amount: decimal.MustParse("0.2")},
		{ID: "quote", Sender: buyer.id, Receiver: seller.id, Instrument: "USDCx", Amount: decimal.MustParse("20.0")},
	}, s.operator)
	sellerSlots := domain.SlotsOf(deadPlan, seller.id)

	for _, slot := range sellerSlots {
		s.allocate(deadID, slot, parties)
	}

	deadCids := s.waitAllocated(deadID, sellerSlots)
	s.waitBalance(seller.id, "CBTC", "0.3", "0.2")

	tokens, err = s.ledger.Tokens(s.ctx, s.operator)
	require.NoError(t, err)

	for _, cid := range deadCids {
		cancel, err := s.ledger.CancelCommand(s.rules, tokens, cid, s.operator)
		require.NoError(t, err)

		_, err = s.ledger.Submit(s.ctx, s.operator, "", cancel)
		require.NoError(t, err)

		// Cancelling it again finds the contract gone.
		_, err = s.ledger.Submit(s.ctx, s.operator, "", cancel)
		require.ErrorIs(t, err, domain.ErrContractGone)
	}

	s.waitBalance(seller.id, "CBTC", "0.5", "0.0")
}

// lockedInputs are the sender's holdings a transfer locked, disclosed by their admin.
func (s *sandbox) lockedInputs(transfer domain.IncomingTransfer) []domain.Disclosed {
	s.t.Helper()

	tokens, err := s.ledger.Tokens(s.ctx, s.operator)
	require.NoError(s.t, err)

	var inputs []domain.Disclosed

	for _, token := range tokens {
		if slices.Contains(transfer.Inputs, token.ContractID) {
			inputs = append(inputs, token)
		}
	}

	require.Len(s.t, inputs, len(transfer.Inputs))

	return inputs
}

func Test_Sandbox_TransferBetweenParties(t *testing.T) {
	s := newSandbox(t)

	alice := s.onboard("sandbox-sender")
	bob := s.onboard("sandbox-receiver")

	s.fund(alice, "CBTC", "1.0")

	holdings, err := s.ledger.Holdings(s.ctx, alice.id)
	require.NoError(t, err)

	inputs, err := domain.SelectHoldings(holdings, alice.id, s.operator, "CBTC", decimal.MustParse("0.4"), time.Now())
	require.NoError(t, err)

	withdraw, err := s.ledger.TransferCommand(s.rules, domain.TransferSpec{
		Admin: s.operator, Sender: alice.id, Receiver: bob.id, InstrumentID: "CBTC",
		Amount: decimal.MustParse("0.4"), Inputs: inputs,
		RequestedAt: time.Now().Add(-time.Minute), ExecuteBefore: time.Now().Add(24 * time.Hour),
	})
	require.NoError(t, err)

	s.sign(alice, withdraw)
	s.waitBalance(alice.id, "CBTC", "0.6", "0.4")

	var incoming []domain.IncomingTransfer

	require.Eventually(t, func() bool {
		incoming, err = s.ledger.TransferInstructions(s.ctx, bob.id)

		return assert.NoError(t, err) && len(incoming) == 1
	}, eventually, tick)
	require.Equal(t, alice.id, incoming[0].Sender)
	require.Equal(t, "0.4", incoming[0].Amount.String())
	require.Len(t, incoming[0].Inputs, 1)

	accept, err := s.ledger.AcceptTransferCommand(s.rules, s.lockedInputs(incoming[0]), incoming[0].ContractID)
	require.NoError(t, err)

	s.sign(bob, accept)

	s.waitBalance(bob.id, "CBTC", "0.4", "0.0")
	s.waitBalance(alice.id, "CBTC", "0.6", "0.0")
}

// Two transactions prepared under one command id (an Allocate retried before the first was
// signed): the ledger executes the first and refuses the second, so no second allocation
// can be funded.
func Test_Sandbox_ExecuteIsIdempotent(t *testing.T) {
	s := newSandbox(t)
	carol := s.onboard("sandbox-carol")

	offer := s.offerMint(carol.id, "CETH", "2.0")

	accept, err := s.ledger.AcceptTransferCommand(s.rules, nil, offer)
	require.NoError(t, err)

	commandID := "accept:" + offer

	first, err := s.ledger.Prepare(s.ctx, carol.id, commandID, accept)
	require.NoError(t, err)

	second, err := s.ledger.Prepare(s.ctx, carol.id, commandID, accept)
	require.NoError(t, err)

	// Signed with another (valid) key, as another user's tab would: rejected for the
	// signature alone, and the very same prepared transaction stays good for carol's key.
	_, otherKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	err = s.ledger.Execute(s.ctx, first, carol.id, ed25519.Sign(otherKey, first.Hash))
	require.ErrorIs(t, err, domain.ErrSignatureRejected)
	require.NotErrorIs(t, err, domain.ErrLedgerUnavailable)

	err = s.ledger.Execute(s.ctx, first, carol.id, make([]byte, ed25519.SignatureSize))
	require.ErrorIs(t, err, domain.ErrSignatureRejected)

	require.NoError(t, s.ledger.Execute(s.ctx, first, carol.id, ed25519.Sign(carol.key, first.Hash)))

	// executeAndWait has committed: the holding is already there, no polling needed.
	holdings, err := s.ledger.Holdings(s.ctx, carol.id)
	require.NoError(t, err)
	require.Equal(t, "2.0", domain.Balances(holdings, carol.id, s.operator, []domain.Instrument{{Symbol: "CETH"}},
		time.Now())[0].Available.String())

	err = s.ledger.Execute(s.ctx, second, carol.id, ed25519.Sign(carol.key, second.Hash))
	require.ErrorIs(t, err, domain.ErrDuplicateCommand)
}

// An allocation that is never settled nor cancelled keeps its holding locked until the
// settlement deadline. After it, the lock has lapsed: the holding shows as available and the
// ledger accepts it as an unlocked input.
func Test_Sandbox_LapsedLockIsSpendable(t *testing.T) {
	s := newSandbox(t)

	dan := s.onboard("sandbox-dan")
	erin := s.onboard("sandbox-erin")

	s.fund(dan, "CBTC", "1.0")

	deadline := time.Now().Add(5 * time.Second)
	plan := domain.Plan([]domain.TransferLeg{
		{ID: "base", Sender: dan.id, Receiver: erin.id, Instrument: "CBTC", Amount: decimal.MustParse("0.4")},
	}, s.operator)

	holdings, err := s.ledger.Holdings(s.ctx, dan.id)
	require.NoError(t, err)

	inputs, err := domain.SelectHoldings(holdings, dan.id, s.operator, "CBTC", decimal.MustParse("0.4"), time.Now())
	require.NoError(t, err)

	allocate, err := s.ledger.AllocateCommand(s.rules, domain.AllocateSpec{
		Operator: s.operator, SettlementID: uuid.NewString(), Slot: domain.SlotsOf(plan, dan.id)[0],
		Deadline: deadline, RequestedAt: time.Now().Add(-time.Minute), Inputs: inputs,
	})
	require.NoError(t, err)

	s.sign(dan, allocate)

	holdings, err = s.ledger.Holdings(s.ctx, dan.id)
	require.NoError(t, err)

	lockedUntil := time.Time{}

	for _, h := range holdings {
		if h.Locked {
			lockedUntil = h.LockedUntil
		}
	}

	require.WithinDuration(t, deadline, lockedUntil, time.Millisecond, "the lock lapses at the settlement deadline")
	require.Equal(t, "0.6", domain.Balances(holdings, dan.id, s.operator, []domain.Instrument{{Symbol: "CBTC"}},
		time.Now())[0].Available.String())

	s.waitBalance(dan.id, "CBTC", "1.0", "0.0")

	holdings, err = s.ledger.Holdings(s.ctx, dan.id)
	require.NoError(t, err)

	inputs, err = domain.SelectHoldings(holdings, dan.id, s.operator, "CBTC", decimal.MustParse("1.0"), time.Now())
	require.NoError(t, err)
	require.Len(t, inputs, 2, "the change and the lapsed-lock holding")

	transfer, err := s.ledger.TransferCommand(s.rules, domain.TransferSpec{
		Admin: s.operator, Sender: dan.id, Receiver: erin.id, InstrumentID: "CBTC", Amount: decimal.MustParse("1.0"),
		Inputs: inputs, RequestedAt: time.Now().Add(-time.Minute), ExecuteBefore: time.Now().Add(time.Hour),
	})
	require.NoError(t, err)

	s.sign(dan, transfer)

	// The whole 1.0 is locked for the transfer to erin now.
	s.waitBalance(dan.id, "CBTC", "0.0", "1.0")
}

func Test_Sandbox_DuplicateCommand(t *testing.T) {
	s := newSandbox(t)
	bob := s.onboard("sandbox-bob")

	// A nonconsuming choice still interprets on a re-submission, so the dedup answers.
	cmd, err := s.ledger.OfferMintCommand(s.rules, domain.OfferMintSpec{
		Admin: s.operator, Receiver: bob.id, InstrumentID: "CETH", Amount: decimal.MustParse("1"),
		OfferedAt: time.Now().Add(-time.Minute),
	})
	require.NoError(t, err)

	commandID := "faucet:" + uuid.NewString()

	_, err = s.ledger.Submit(s.ctx, s.operator, commandID, cmd)
	require.NoError(t, err)

	_, err = s.ledger.Submit(s.ctx, s.operator, commandID, cmd)
	require.ErrorIs(t, err, domain.ErrDuplicateCommand)
	require.ErrorIs(t, err, domain.ErrLedger)
}

func Test_Sandbox_ResolveParty(t *testing.T) {
	s := newSandbox(t)

	makerA, err := s.ledger.ResolveParty(s.ctx, "makerA")
	require.NoError(t, err)
	require.Equal(t, "makerA", domain.Hint(makerA))

	// "maker" is a prefix of makerA and makerB, but no party's hint.
	_, err = s.ledger.ResolveParty(s.ctx, "maker")
	require.ErrorIs(t, err, domain.ErrNotFound)
}
