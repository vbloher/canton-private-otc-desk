package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/domain"
)

//go:generate go tool mockery --config=../../.mockery.yml

const (
	registrationTTL = 10 * time.Minute
	challengeTTL    = 5 * time.Minute
	sessionCacheTTL = time.Minute
)

type Postgres interface {
	CreateUser(ctx context.Context, user domain.User) error
	UpsertHouseUser(ctx context.Context, user domain.User) error
	GetUser(ctx context.Context, id uuid.UUID) (domain.User, error)
	GetUserByCredential(ctx context.Context, credentialID string) (domain.User, error)
	GetUserByAPIKeyHash(ctx context.Context, hash string) (domain.User, error)
	GetUserBySession(ctx context.Context, tokenHash string, now time.Time) (domain.User, error)
	ListMakers(ctx context.Context) ([]domain.User, error)
	SetMakerMode(ctx context.Context, id uuid.UUID, enabled bool) (domain.User, error)
	CreateSession(ctx context.Context, tokenHash string, userID uuid.UUID, expiresAt time.Time) error
	DeleteSession(ctx context.Context, tokenHash string) error
}

type Redis interface {
	SaveRegistration(ctx context.Context, r domain.Registration, ttl time.Duration) error
	// TakeRegistration reads and deletes in one step (GETDEL); ErrRegistrationExpired when it is gone.
	TakeRegistration(ctx context.Context, id uuid.UUID) (domain.Registration, error)
	SaveChallenge(ctx context.Context, challenge string, userID uuid.UUID, ttl time.Duration) error
	// TakeChallenge reads and deletes in one step (GETDEL); ErrUnauthorized when it is gone.
	TakeChallenge(ctx context.Context, challenge string) (uuid.UUID, error)
	GetCachedSession(ctx context.Context, tokenHash string) (domain.User, bool)
	CacheSession(ctx context.Context, tokenHash string, user domain.User, ttl time.Duration)
	DropCachedSession(ctx context.Context, tokenHash string)
}

type Ledger interface {
	ResolveParty(ctx context.Context, hint string) (string, error)
	GenerateExternalParty(ctx context.Context, hint string, publicKey []byte) (domain.Topology, error)
	AllocateExternalParty(ctx context.Context, t domain.Topology, signature []byte) (string, error)
}

type Config struct {
	SessionTTL  time.Duration      `env:"SESSION_TTL"  envDefault:"168h"`
	HouseMakers domain.HouseMakers `env:"HOUSE_MAKERS" envDefault:"makerA:demo-key-a,makerB:demo-key-b"`
}

type UseCase struct {
	postgres Postgres
	redis    Redis
	ledger   Ledger
	config   Config
}

func New(postgres Postgres, redis Redis, ledger Ledger, config Config) *UseCase {
	return &UseCase{
		postgres: postgres,
		redis:    redis,
		ledger:   ledger,
		config:   config,
	}
}

// startSession stores a new session of the user and returns its token and the expiry that was stored.
func (u *UseCase) startSession(ctx context.Context, userID uuid.UUID) (token string, expiresAt time.Time, err error) {
	token, hash := domain.NewSessionToken()
	expiresAt = time.Now().Add(u.config.SessionTTL)

	err = u.postgres.CreateSession(ctx, hash, userID, expiresAt)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("postgres.CreateSession: %w", err)
	}

	return token, expiresAt, nil
}
