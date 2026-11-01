package bot

import (
	"context"
	"crypto/ed25519"
	"math/big"
	"time"

	"github.com/google/uuid"

	"github.com/vbloher/canton-private-otc-desk/maker-bot/internal/desk"
)

type Account interface {
	SetSigner(partyID string, sign func(message []byte) []byte)
	StartRegistration(ctx context.Context, displayName string, publicKey ed25519.PublicKey) (desk.Registration, error)
	RegisterParty(ctx context.Context, registrationID uuid.UUID, signature []byte) error
	CompleteRegistration(ctx context.Context, registrationID uuid.UUID, signature []byte) (desk.Me, error)
	SignIn(ctx context.Context) error
	SetMakerMode(ctx context.Context, enabled bool) error
}

type Trading interface {
	IncomingRFQs(ctx context.Context) ([]desk.IncomingRFQ, error)
	SubmitQuote(ctx context.Context, rfqID uuid.UUID, price string, ttl time.Duration) (desk.Quote, error)
}

type Wallet interface {
	PendingActions(ctx context.Context) ([]desk.Action, error)
	Execute(ctx context.Context, actionID uuid.UUID, signature []byte) error
	Balances(ctx context.Context) (map[string]*big.Rat, error)
	IncomingTransfers(ctx context.Context) ([]desk.Transfer, error)
	AcceptTransfer(ctx context.Context, contractID string) error
	Network(ctx context.Context) (desk.Network, error)
	Faucet(ctx context.Context) error
}

type Desk interface {
	Account
	Trading
	Wallet
}

var _ Desk = (*desk.Client)(nil)
