package usecase_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"

	"github.com/vbloher/canton-private-otc-desk/pkg/events"
	"github.com/vbloher/canton-private-otc-desk/pkg/otel"
	"github.com/vbloher/canton-private-otc-desk/services/notification/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/notification/internal/usecase"
	"github.com/vbloher/canton-private-otc-desk/services/notification/internal/usecase/mocks"
)

const (
	Any       = mock.Anything
	alice     = "alice-3f9a1c::1220ab"
	bob       = "bob-77aa01::1220cd"
	publicURL = "https://desk.example.com"
)

var testConfig = usecase.Config{PublicURL: publicURL, ConfirmationTTL: 24 * time.Hour, Enabled: true}

type inTransaction struct{}

type deps struct {
	postgres *mocks.Postgres
	redis    *mocks.Redis
	sender   *mocks.Sender
	contacts *mocks.Contacts
	events   *mocks.Events
}

func newDeps(t *testing.T) deps {
	t.Helper()

	otel.SilentModeInit()

	return deps{
		postgres: mocks.NewPostgres(t),
		redis:    mocks.NewRedis(t),
		sender:   mocks.NewSender(t),
		contacts: mocks.NewContacts(t),
		events:   mocks.NewEvents(t),
	}
}

func (d deps) useCase(t *testing.T, c usecase.Config) *usecase.UseCase {
	t.Helper()

	tx := mocks.NewTransactor(t)
	tx.EXPECT().Wrap(Any, Any).RunAndReturn(func(ctx context.Context, fn func(context.Context) error) error {
		return fn(context.WithValue(ctx, inTransaction{}, true))
	}).Maybe()

	return usecase.New(c, tx, d.postgres, d.redis, d.sender, d.contacts, d.events)
}

func inTx() any {
	return mock.MatchedBy(func(ctx context.Context) bool {
		inside, _ := ctx.Value(inTransaction{}).(bool)

		return inside
	})
}

func profileEvent() events.Event {
	return events.Event{Type: events.ProfileUpdated, Data: struct{}{}}
}

func confirmedProfile(party, email string) domain.Profile {
	at := time.Now()

	return domain.Profile{PartyID: party, Email: email, ConfirmedAt: &at}
}
