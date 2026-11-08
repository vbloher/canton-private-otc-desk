package usecase

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/rs/zerolog/log"

	"github.com/vbloher/canton-private-otc-desk/pkg/events"
	"github.com/vbloher/canton-private-otc-desk/services/notification/internal/domain"
)

//go:generate go tool mockery --config=../../.mockery.yml

//nolint:gochecknoglobals
var emailsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "notification_emails_total",
	Help: "Total number of attempts to send an email, by kind and outcome",
}, []string{"kind", "status"})

type Config struct {
	PublicURL       string        `env:"PUBLIC_URL"`
	ConfirmationTTL time.Duration `env:"EMAIL_CONFIRMATION_TTL"  envDefault:"24h"`
	IncludeUpdateID bool          `env:"EMAIL_INCLUDE_UPDATE_ID" envDefault:"false"`
	Enabled         bool          `env:"-"`
}

func (c Config) baseURL() string {
	return strings.TrimRight(c.PublicURL, "/")
}

type Transactor interface {
	Wrap(ctx context.Context, fn func(ctx context.Context) error) error
}

//nolint:interfacebloat
type Postgres interface {
	GetProfile(ctx context.Context, partyID string) (domain.Profile, error)
	GetProfileForUpdate(ctx context.Context, partyID string) (domain.Profile, error)
	ListProfiles(ctx context.Context, partyIDs []string) ([]domain.Profile, error)
	SaveProfile(ctx context.Context, profile domain.Profile) error
	NewsWanted(ctx context.Context, email string) (bool, error)
	ClearNews(ctx context.Context, email string) ([]string, error)

	ReplaceConfirmation(ctx context.Context, c domain.Confirmation) error
	GetConfirmation(ctx context.Context, tokenHash string) (domain.Confirmation, error)
	MarkConfirmed(ctx context.Context, tokenHash string, at time.Time) error

	CreateEmail(ctx context.Context, e domain.Email) error
	NextDueEmail(ctx context.Context, now time.Time) (domain.Email, error)
	UpdateEmail(ctx context.Context, e domain.Email) error

	QueueContactSync(ctx context.Context, email string, now time.Time) error
	NextDueContactSync(ctx context.Context, now time.Time) (domain.ContactSync, error)
	RescheduleContactSync(ctx context.Context, s domain.ContactSync) error
	DeleteContactSync(ctx context.Context, email string) error

	Cleanup(ctx context.Context, now, finishedBefore time.Time) error
}

type Redis interface {
	AllowConfirmation(ctx context.Context, partyID, email string) (bool, error)
}

type Sender interface {
	Send(ctx context.Context, e domain.Email) (string, error)
}

type Contacts interface {
	Subscribe(ctx context.Context, email string) error
	Unsubscribe(ctx context.Context, email string) error
	ParseWebhook(payload []byte, id, timestamp, signature string) (domain.ContactEvent, error)
}

type Events interface {
	Publish(ctx context.Context, ev events.Event, partyIDs ...string) error
}

type UseCase struct {
	config   Config
	tx       Transactor
	postgres Postgres
	redis    Redis
	sender   Sender
	contacts Contacts
	events   Events

	cleaned time.Time
}

func New(
	c Config, tx Transactor, postgres Postgres, redis Redis, sender Sender, contacts Contacts, publisher Events,
) *UseCase {
	return &UseCase{
		config:   c,
		tx:       tx,
		postgres: postgres,
		redis:    redis,
		sender:   sender,
		contacts: contacts,
		events:   publisher,
	}
}

func (u *UseCase) publishProfile(ctx context.Context, partyIDs ...string) {
	ev := events.Event{Type: events.ProfileUpdated, Data: struct{}{}}

	err := u.events.Publish(context.WithoutCancel(ctx), ev, partyIDs...)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Msg("usecase: events.Publish")
	}
}

func orEmpty(partyID string, profile domain.Profile, err error) (domain.Profile, error) {
	if errors.Is(err, domain.ErrNotFound) {
		return domain.Profile{PartyID: partyID}, nil
	}

	return profile, err
}
