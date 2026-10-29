package usecase

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/vbloher/canton-private-otc-desk/pkg/events"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/dto"
)

//go:generate go tool mockery --config=../../.mockery.yml

//nolint:interfacebloat
type Canton interface {
	Ready(ctx context.Context) bool
	ResolveParty(ctx context.Context, hint string) (string, error)
	SynchronizerID(ctx context.Context, party string) (string, error)
	GenerateExternalParty(ctx context.Context, synchronizerID, hint string, publicKey []byte) (domain.Topology, error)
	AllocateExternalParty(ctx context.Context, synchronizerID string, transactions []string, signature []byte,
		fingerprint string) (string, error)
	Holdings(ctx context.Context, party string) ([]domain.Holding, error)
	Allocations(ctx context.Context, executor string) ([]domain.Allocation, error)
	TransferInstructions(ctx context.Context, receiver string) ([]domain.IncomingTransfer, error)
	Submit(ctx context.Context, actAs, commandID string, cmd domain.Command) (domain.Transaction, error)
	Prepare(ctx context.Context, actAs, commandID string, cmd domain.Command) (domain.Prepared, error)
	Execute(ctx context.Context, prepared domain.Prepared, party string, signature []byte) error
}

type Registry interface {
	AllocateCommand(ctx context.Context, in domain.AllocateSpec) (domain.Command, error)
	SettleCommand(ctx context.Context, in domain.SettleSpec) (domain.Command, error)
	CancelCommand(ctx context.Context, registry, allocationCID, executor string) (domain.Command, error)
	TransferCommand(ctx context.Context, in domain.TransferSpec) (domain.Command, error)
	AcceptTransferCommand(ctx context.Context, registry, transferCID string) (domain.Command, error)
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
	GetSynchronizerID(ctx context.Context) (string, bool)
	SetSynchronizerID(ctx context.Context, id string)
	ClaimFaucet(ctx context.Context, party string, cooldown time.Duration) (bool, error)
	ReleaseFaucet(ctx context.Context, party string)
}

type Events interface {
	Publish(ctx context.Context, ev events.Event, partyIDs ...string) error
}

type Transactor interface {
	Wrap(ctx context.Context, fn func(ctx context.Context) error) error
}

type Config struct {
	Network        domain.Network        `env:"CANTON_NETWORK"          envDefault:"devnet"`
	OperatorParty  string                `env:"OPERATOR_PARTY,notEmpty"`
	MakerParty     string                `env:"MAKER_PARTY"`
	Instruments    domain.InstrumentList `env:"INSTRUMENTS,notEmpty"`
	FaucetParty    string                `env:"FAUCET_PARTY"`
	FaucetAmounts  domain.Amounts        `env:"FAUCET_AMOUNTS"`
	FaucetCooldown time.Duration         `env:"FAUCET_COOLDOWN"         envDefault:"1h"`
	ActionTTL      time.Duration         `env:"ACTION_TTL"              envDefault:"10m"`

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

type UseCase struct {
	config   Config
	tx       Transactor
	postgres Postgres
	redis    Redis
	canton   Canton
	registry Registry
	events   Events

	operator string
	faucet   string
	hosted   map[string]bool
	faucetMu sync.Mutex
}

func New(c Config, tx Transactor, postgres Postgres, redis Redis, canton Canton, registry Registry, bus Events,
) *UseCase {
	return &UseCase{
		config:   c,
		tx:       tx,
		postgres: postgres,
		redis:    redis,
		canton:   canton,
		registry: registry,
		events:   bus,
		hosted:   map[string]bool{},
	}
}

func (c Config) faucetEnabled() bool {
	return c.Network == domain.NetworkDevNet && c.FaucetParty != "" && len(c.FaucetAmounts) > 0
}

const clockSkew = time.Minute

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

func (u *UseCase) publish(ctx context.Context, ev events.Event, partyIDs ...string) {
	err := u.events.Publish(ctx, ev, partyIDs...)
	if err != nil {
		log.Error().Err(err).Str("type", ev.Type).Msg("usecase: events.Publish")
	}
}

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

func (u *UseCase) legs(in []dto.TransferLeg) ([]domain.TransferLeg, error) {
	if len(in) == 0 {
		return nil, domain.Errorf(domain.ErrValidation, "a settlement needs legs")
	}

	legs := make([]domain.TransferLeg, 0, len(in))

	for _, l := range in {
		inst, ok := u.config.Instruments.Find(l.Instrument)
		if !ok && l.Instrument != "" {
			return nil, domain.Errorf(domain.ErrUnknownInstrument, "%q", l.Instrument)
		}

		leg, err := domain.NewTransferLeg(l.ID, l.Sender, l.Receiver, inst, l.Amount)
		if err != nil {
			return nil, fmt.Errorf("domain.NewTransferLeg: %w", err)
		}

		legs = append(legs, leg)
	}

	return legs, nil
}
