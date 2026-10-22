package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/vbloher/canton-private-otc-desk/pkg/events"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/dto"
)

//go:generate go tool mockery --config=../../.mockery.yml

// Canton is every interaction with the ledger, one port by design (the brief's contract).
//
//nolint:interfacebloat // splitting it would only spread one adapter over several ports
type Canton interface {
	Ready(ctx context.Context) bool
	ResolveParty(ctx context.Context, hint string) (string, error) // ErrNotFound when absent
	SynchronizerID(ctx context.Context, party string) (string, error)
	GenerateExternalParty(ctx context.Context, synchronizerID, hint string, publicKey []byte) (domain.Topology, error)
	AllocateExternalParty(ctx context.Context, synchronizerID string, transactions []string, signature []byte,
		fingerprint string) (string, error)
	Holdings(ctx context.Context, party string) ([]domain.Holding, error)
	Allocations(ctx context.Context, executor string) ([]domain.Allocation, error)
	TransferInstructions(ctx context.Context, receiver string) ([]domain.IncomingTransfer, error)
	TokenRules(ctx context.Context, admin string) (domain.Disclosed, error)
	Tokens(ctx context.Context, admin string) ([]domain.Disclosed, error)
	AllocateCommand(rules domain.Disclosed, in domain.AllocateSpec) (domain.Command, error)
	SettleCommand(rules domain.Disclosed, tokens []domain.Disclosed, in domain.SettleSpec) (domain.Command, error)
	CancelCommand(rules domain.Disclosed, tokens []domain.Disclosed, allocationCID, executor string) (
		domain.Command, error)
	AcceptTransferCommand(rules domain.Disclosed, inputs []domain.Disclosed, transferCID string) (domain.Command, error)
	TransferCommand(rules domain.Disclosed, in domain.TransferSpec) (domain.Command, error)
	OfferMintCommand(rules domain.Disclosed, in domain.OfferMintSpec) (domain.Command, error)
	Submit(ctx context.Context, actAs, commandID string, cmd domain.Command) (domain.Transaction, error)
	Prepare(ctx context.Context, actAs, commandID string, cmd domain.Command) (domain.Prepared, error)
	Execute(ctx context.Context, prepared domain.Prepared, party string, signature []byte) error
}

type Postgres interface {
	SaveActions(ctx context.Context, actions ...domain.Action) error
	GetActionForUpdate(ctx context.Context, id uuid.UUID) (domain.Action, error)
	UpdateAction(ctx context.Context, action domain.Action) error
	ListPendingActions(ctx context.Context, partyID string, now time.Time) ([]domain.Action, error)
	ExpireActions(ctx context.Context, now time.Time) ([]domain.Action, error)
	ExpireActionsByRef(ctx context.Context, ref string) ([]domain.Action, error)
}

type Redis interface {
	GetTokenRules(ctx context.Context) (domain.Disclosed, bool)
	SetTokenRules(ctx context.Context, rules domain.Disclosed)
	GetSynchronizerID(ctx context.Context) (string, bool)
	SetSynchronizerID(ctx context.Context, id string)
}

type Events interface {
	Publish(ctx context.Context, ev events.Event, partyIDs ...string) error
}

type Transactor interface {
	Wrap(ctx context.Context, fn func(ctx context.Context) error) error
}

type Config struct {
	OperatorHint  string                `env:"OPERATOR_HINT"  envDefault:"operator"`
	HouseParties  []string              `env:"HOUSE_PARTIES"  envDefault:"makerA,makerB"`
	Instruments   domain.InstrumentList `env:"INSTRUMENTS"    envDefault:"CBTC:Bitcoin:8,USDCx:USD Coin:2,CETH:Ether:6"`
	FaucetAmounts domain.Amounts        `env:"FAUCET_AMOUNTS" envDefault:"CBTC:1,USDCx:100000,CETH:10"`
	ActionTTL     time.Duration         `env:"ACTION_TTL"     envDefault:"10m"`

	// StartupTimeout bounds how long Init waits for the ledger and StartupInterval paces its
	// polls; zero means 120 s and 1 s. They are not read from the environment.
	StartupTimeout  time.Duration
	StartupInterval time.Duration
}

const (
	defaultStartupTimeout  = 120 * time.Second
	defaultStartupInterval = time.Second
)

func (c Config) startupTimeout() time.Duration {
	if c.StartupTimeout <= 0 {
		return defaultStartupTimeout
	}

	return c.StartupTimeout
}

func (c Config) startupInterval() time.Duration {
	if c.StartupInterval <= 0 {
		return defaultStartupInterval
	}

	return c.StartupInterval
}

// UseCase is everything the desk does on Canton. Init resolves the operator and the parties
// the participant hosts (operator ∪ HOUSE_PARTIES); every other party is external and signs
// its own transactions.
type UseCase struct {
	config   Config
	tx       Transactor
	postgres Postgres
	redis    Redis
	canton   Canton
	events   Events

	operator string
	hosted   map[string]bool
}

func New(c Config, tx Transactor, postgres Postgres, redis Redis, canton Canton, bus Events) *UseCase {
	return &UseCase{
		config:   c,
		tx:       tx,
		postgres: postgres,
		redis:    redis,
		canton:   canton,
		events:   bus,
		hosted:   map[string]bool{},
	}
}

// clockSkew backdates the times the ledger requires to be in the past (requestedAt,
// offeredAt, a lapsed lock's expiry), so a clock slightly ahead of the participant's cannot
// fail a command.
const clockSkew = time.Minute

// locksAsOf is when holding locks are judged: a lock counts as lapsed only once it has
// lapsed by the participant's clock too. Balances use it as well, so whatever shows as
// available can be spent.
func locksAsOf() time.Time {
	return time.Now().Add(-clockSkew)
}

type actionEvent struct {
	ActionID uuid.UUID `json:"actionId"`
}

func signRequested(id uuid.UUID) events.Event {
	return events.Event{Type: events.SignRequested, Data: actionEvent{ActionID: id}}
}

func signCompleted(id uuid.UUID) events.Event {
	return events.Event{Type: events.SignCompleted, Data: actionEvent{ActionID: id}}
}

func walletUpdated() events.Event {
	return events.Event{Type: events.WalletUpdated, Data: struct{}{}}
}

// publish is best effort: the bus is at-most-once and clients re-fetch over REST.
func (u *UseCase) publish(ctx context.Context, ev events.Event, partyIDs ...string) {
	err := u.events.Publish(ctx, ev, partyIDs...)
	if err != nil {
		log.Error().Err(err).Str("type", ev.Type).Msg("usecase: events.Publish")
	}
}

// tokenRules is the sandbox registry's factory, cached in Redis.
func (u *UseCase) tokenRules(ctx context.Context) (domain.Disclosed, error) {
	rules, ok := u.redis.GetTokenRules(ctx)
	if ok {
		return rules, nil
	}

	rules, err := u.canton.TokenRules(ctx, u.operator)
	if err != nil {
		return domain.Disclosed{}, fmt.Errorf("canton.TokenRules: %w", err)
	}

	u.redis.SetTokenRules(ctx, rules)

	return rules, nil
}

// synchronizerID pins external-party onboarding, cached in Redis.
func (u *UseCase) synchronizerID(ctx context.Context) (string, error) {
	id, ok := u.redis.GetSynchronizerID(ctx)
	if ok {
		return id, nil
	}

	id, err := u.canton.SynchronizerID(ctx, u.operator)
	if err != nil {
		return "", fmt.Errorf("canton.SynchronizerID: %w", err)
	}

	u.redis.SetSynchronizerID(ctx, id)

	return id, nil
}

// legs validates a settlement's legs: positive amounts, known instruments.
func (u *UseCase) legs(in []dto.TransferLeg) ([]domain.TransferLeg, error) {
	if len(in) == 0 {
		return nil, domain.Errorf(domain.ErrValidation, "a settlement needs legs")
	}

	legs := make([]domain.TransferLeg, 0, len(in))

	for _, l := range in {
		leg, err := domain.NewTransferLeg(l.ID, l.Sender, l.Receiver, l.Instrument, l.Amount)
		if err != nil {
			return nil, fmt.Errorf("domain.NewTransferLeg: %w", err)
		}

		if _, ok := u.config.Instruments.Find(leg.Instrument); !ok {
			return nil, domain.Errorf(domain.ErrUnknownInstrument, "%q", leg.Instrument)
		}

		legs = append(legs, leg)
	}

	return legs, nil
}
