package bot_test

import (
	"context"
	"crypto/ed25519"
	"math/big"
	"slices"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/vbloher/canton-private-otc-desk/maker-bot/internal/desk"
)

type submission struct {
	RFQID uuid.UUID
	Price string
	TTL   time.Duration
}

type execution struct {
	ID        uuid.UUID
	Signature []byte
}

type fakeDesk struct {
	mu    sync.Mutex
	calls []string

	registration      desk.Registration
	registerErrs      []error
	me                desk.Me
	signInErr         error
	onRegister        func()
	displayName       string
	publicKey         ed25519.PublicKey
	registerSignature []byte
	signerParty       string

	makerMode []bool

	incoming    []desk.IncomingRFQ
	incomingErr error
	submissions []submission
	submitErrs  []error

	pending     []desk.Action
	executions  []execution
	executeErrs map[uuid.UUID][]error

	balances    map[string]*big.Rat
	transfers   []desk.Transfer
	accepted    []string
	network     desk.Network
	faucetErr   error
	faucetCalls int
}

func (f *fakeDesk) called(name string) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.calls = append(f.calls, name)
}

func (f *fakeDesk) callNames() []string {
	f.mu.Lock()
	defer f.mu.Unlock()

	return slices.Clone(f.calls)
}

func (f *fakeDesk) SetSigner(partyID string, _ func(message []byte) []byte) {
	f.called("SetSigner")

	f.mu.Lock()
	defer f.mu.Unlock()

	f.signerParty = partyID
}

func (f *fakeDesk) StartRegistration(_ context.Context, displayName string, publicKey ed25519.PublicKey) (desk.Registration, error) {
	f.called("StartRegistration")

	f.mu.Lock()
	defer f.mu.Unlock()

	f.displayName, f.publicKey = displayName, publicKey

	return f.registration, nil
}

func (f *fakeDesk) RegisterParty(_ context.Context, registrationID uuid.UUID, signature []byte) error {
	f.called("RegisterParty")

	if f.onRegister != nil {
		f.onRegister()
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	f.registerSignature = signature

	if len(f.registerErrs) > 0 {
		err := f.registerErrs[0]
		f.registerErrs = f.registerErrs[1:]

		return err
	}

	if registrationID != f.registration.ID {
		return desk.ErrGone
	}

	return nil
}

func (f *fakeDesk) CompleteRegistration(context.Context, uuid.UUID, []byte) (desk.Me, error) {
	f.called("CompleteRegistration")

	return f.me, nil
}

func (f *fakeDesk) SignIn(context.Context) error {
	f.called("SignIn")

	return f.signInErr
}

func (f *fakeDesk) SetMakerMode(_ context.Context, enabled bool) error {
	f.called("SetMakerMode")

	f.mu.Lock()
	defer f.mu.Unlock()

	f.makerMode = append(f.makerMode, enabled)

	return nil
}

func (f *fakeDesk) IncomingRFQs(context.Context) ([]desk.IncomingRFQ, error) {
	f.called("IncomingRFQs")

	f.mu.Lock()
	defer f.mu.Unlock()

	return slices.Clone(f.incoming), f.incomingErr
}

func (f *fakeDesk) SubmitQuote(_ context.Context, rfqID uuid.UUID, price string, ttl time.Duration) (desk.Quote, error) {
	f.called("SubmitQuote")

	f.mu.Lock()
	defer f.mu.Unlock()

	f.submissions = append(f.submissions, submission{RFQID: rfqID, Price: price, TTL: ttl})

	if len(f.submitErrs) > 0 {
		err := f.submitErrs[0]
		f.submitErrs = f.submitErrs[1:]

		if err != nil {
			return desk.Quote{}, err
		}
	}

	return desk.Quote{ID: uuid.New(), Status: "allocating"}, nil
}

func (f *fakeDesk) PendingActions(context.Context) ([]desk.Action, error) {
	f.called("PendingActions")

	f.mu.Lock()
	defer f.mu.Unlock()

	return slices.Clone(f.pending), nil
}

func (f *fakeDesk) Execute(_ context.Context, actionID uuid.UUID, signature []byte) error {
	f.called("Execute")

	f.mu.Lock()
	defer f.mu.Unlock()

	f.executions = append(f.executions, execution{ID: actionID, Signature: signature})

	if errs := f.executeErrs[actionID]; len(errs) > 0 {
		f.executeErrs[actionID] = errs[1:]

		return errs[0]
	}

	return nil
}

func (f *fakeDesk) Balances(context.Context) (map[string]*big.Rat, error) {
	f.called("Balances")

	f.mu.Lock()
	defer f.mu.Unlock()

	return f.balances, nil
}

func (f *fakeDesk) IncomingTransfers(context.Context) ([]desk.Transfer, error) {
	f.called("IncomingTransfers")

	f.mu.Lock()
	defer f.mu.Unlock()

	return slices.Clone(f.transfers), nil
}

func (f *fakeDesk) AcceptTransfer(_ context.Context, contractID string) error {
	f.called("AcceptTransfer")

	f.mu.Lock()
	defer f.mu.Unlock()

	f.accepted = append(f.accepted, contractID)

	return nil
}

func (f *fakeDesk) Network(context.Context) (desk.Network, error) {
	f.called("Network")

	f.mu.Lock()
	defer f.mu.Unlock()

	return f.network, nil
}

func (f *fakeDesk) Faucet(context.Context) error {
	f.called("Faucet")

	f.mu.Lock()
	defer f.mu.Unlock()

	f.faucetCalls++

	return f.faucetErr
}
