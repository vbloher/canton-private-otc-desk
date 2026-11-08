package http_test

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	nethttp "net/http"

	"context"

	"github.com/vbloher/canton-private-otc-desk/pkg/metrics"
	"github.com/vbloher/canton-private-otc-desk/pkg/otel"
	"github.com/vbloher/canton-private-otc-desk/pkg/principal"
	"github.com/vbloher/canton-private-otc-desk/pkg/router"
	controller "github.com/vbloher/canton-private-otc-desk/services/notification/internal/controller/http"
	"github.com/vbloher/canton-private-otc-desk/services/notification/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/notification/internal/usecase"
	"github.com/vbloher/canton-private-otc-desk/services/notification/internal/usecase/mocks"
)

const (
	Any   = mock.Anything
	party = "alice-3f9a1c::1220ab"
)

var sharedMetrics = sync.OnceValue(metrics.NewHTTPServer)

type server struct {
	handler  nethttp.Handler
	postgres *mocks.Postgres
	redis    *mocks.Redis
	contacts *mocks.Contacts
	events   *mocks.Events
}

func newServer(t *testing.T, enabled bool) server {
	t.Helper()

	otel.SilentModeInit()

	s := server{
		postgres: mocks.NewPostgres(t),
		redis:    mocks.NewRedis(t),
		contacts: mocks.NewContacts(t),
		events:   mocks.NewEvents(t),
	}

	tx := mocks.NewTransactor(t)
	tx.EXPECT().Wrap(Any, Any).RunAndReturn(func(ctx context.Context, fn func(context.Context) error) error {
		return fn(ctx)
	}).Maybe()

	uc := usecase.New(usecase.Config{
		PublicURL: "https://desk.example.com", ConfirmationTTL: time.Hour, Enabled: enabled,
	}, tx, s.postgres, s.redis, mocks.NewSender(t), s.contacts, s.events)

	r := router.New()
	require.NoError(t, controller.NotificationRouter(r, uc, sharedMetrics()))

	s.handler = r

	return s
}

func (s server) do(method, path, body string, signedIn bool) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")

	if signedIn {
		principal.Principal{UserID: uuid.New(), PartyID: party, Hint: "alice-3f9a1c"}.SetHeaders(r.Header)
	}

	w := httptest.NewRecorder()
	s.handler.ServeHTTP(w, r)

	return w
}

func jsonBody(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()

	var body map[string]any

	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body), w.Body.String())

	return body
}

func requireError(t *testing.T, w *httptest.ResponseRecorder, status int, message string) {
	t.Helper()

	require.Equal(t, status, w.Code, w.Body.String())
	require.Equal(t, message, jsonBody(t, w)["error"])
}

func Test_GetProfile_RequiresTheCaller(t *testing.T) {
	s := newServer(t, true)

	require.Equal(t, nethttp.StatusUnauthorized, s.do("GET", "/api/v1/profile", "", false).Code)
}

func Test_GetProfile_ReturnsTheProfile(t *testing.T) {
	s := newServer(t, true)
	at := time.Now()

	s.postgres.EXPECT().GetProfile(Any, party).
		Return(domain.Profile{PartyID: party, Email: "a@b.co", ConfirmedAt: &at, News: true}, nil)

	w := s.do("GET", "/api/v1/profile", "", true)

	require.Equal(t, nethttp.StatusOK, w.Code, w.Body.String())
	require.JSONEq(t, `{"email":"a@b.co","emailStatus":"confirmed","news":true,"trades":false,"emailEnabled":true}`, w.Body.String())
}

func Test_GetProfile_LeavesTheAddressOutWhenThereIsNone(t *testing.T) {
	s := newServer(t, false)

	s.postgres.EXPECT().GetProfile(Any, party).Return(domain.Profile{}, domain.ErrNotFound)

	w := s.do("GET", "/api/v1/profile", "", true)

	require.Equal(t, nethttp.StatusOK, w.Code, w.Body.String())
	require.JSONEq(t, `{"emailStatus":"none","news":false,"trades":false,"emailEnabled":false}`, w.Body.String())
}

func Test_SetEmail_ReturnsTheUnconfirmedProfile(t *testing.T) {
	s := newServer(t, true)

	s.postgres.EXPECT().GetProfile(Any, party).Return(domain.Profile{}, domain.ErrNotFound)
	s.redis.EXPECT().AllowConfirmation(Any, party, "a@b.co").Return(true, nil)
	s.postgres.EXPECT().GetProfileForUpdate(Any, party).Return(domain.Profile{}, domain.ErrNotFound)
	s.postgres.EXPECT().SaveProfile(Any, Any).Return(nil)
	s.postgres.EXPECT().ReplaceConfirmation(Any, Any).Return(nil)
	s.postgres.EXPECT().CreateEmail(Any, Any).Return(nil)

	w := s.do("PUT", "/api/v1/profile/email", `{"email":"a@b.co"}`, true)

	require.Equal(t, nethttp.StatusOK, w.Code, w.Body.String())
	require.JSONEq(t, `{"email":"a@b.co","emailStatus":"unconfirmed","news":false,"trades":false,"emailEnabled":true}`, w.Body.String())
}

func Test_SetEmail_Errors(t *testing.T) {
	t.Run("without a caller", func(t *testing.T) {
		s := newServer(t, true)

		require.Equal(t, nethttp.StatusUnauthorized, s.do("PUT", "/api/v1/profile/email", `{"email":"a@b.co"}`, false).Code)
	})

	t.Run("an invalid address", func(t *testing.T) {
		s := newServer(t, true)

		requireError(t, s.do("PUT", "/api/v1/profile/email", `{"email":"nope"}`, true),
			nethttp.StatusBadRequest, "Enter a valid email address.")
	})

	t.Run("a body the contract does not allow", func(t *testing.T) {
		s := newServer(t, true)

		require.Equal(t, nethttp.StatusBadRequest, s.do("PUT", "/api/v1/profile/email", `{}`, true).Code)
	})

	t.Run("over the limit", func(t *testing.T) {
		s := newServer(t, true)

		s.postgres.EXPECT().GetProfile(Any, party).Return(domain.Profile{}, domain.ErrNotFound)
		s.redis.EXPECT().AllowConfirmation(Any, party, "a@b.co").Return(false, nil)

		requireError(t, s.do("PUT", "/api/v1/profile/email", `{"email":"a@b.co"}`, true),
			nethttp.StatusTooManyRequests, "Too many confirmation emails requested. Try again later.")
	})

	t.Run("email off", func(t *testing.T) {
		s := newServer(t, false)

		requireError(t, s.do("PUT", "/api/v1/profile/email", `{"email":"a@b.co"}`, true),
			nethttp.StatusServiceUnavailable, "Email isn't set up on this desk.")
	})
}

func Test_SetNotifications_UpdatesAConfirmedProfile(t *testing.T) {
	s := newServer(t, true)
	at := time.Now()

	s.postgres.EXPECT().GetProfileForUpdate(Any, party).
		Return(domain.Profile{PartyID: party, Email: "a@b.co", ConfirmedAt: &at}, nil)
	s.postgres.EXPECT().SaveProfile(Any, Any).Return(nil)
	s.events.EXPECT().Publish(Any, Any, Any).Return(nil)

	w := s.do("PUT", "/api/v1/profile/notifications", `{"news":false,"trades":true}`, true)

	require.Equal(t, nethttp.StatusOK, w.Code, w.Body.String())
	require.JSONEq(t, `{"email":"a@b.co","emailStatus":"confirmed","news":false,"trades":true,"emailEnabled":true}`, w.Body.String())
}

func Test_SetNotifications_Errors(t *testing.T) {
	t.Run("not confirmed", func(t *testing.T) {
		s := newServer(t, true)

		s.postgres.EXPECT().GetProfileForUpdate(Any, party).Return(domain.Profile{PartyID: party, Email: "a@b.co"}, nil)

		requireError(t, s.do("PUT", "/api/v1/profile/notifications", `{"news":true,"trades":true}`, true),
			nethttp.StatusConflict, "Confirm your email first.")
	})

	t.Run("a missing field", func(t *testing.T) {
		s := newServer(t, true)

		require.Equal(t, nethttp.StatusBadRequest, s.do("PUT", "/api/v1/profile/notifications", `{"news":true}`, true).Code)
	})

	t.Run("without a caller", func(t *testing.T) {
		s := newServer(t, true)

		require.Equal(t, nethttp.StatusUnauthorized,
			s.do("PUT", "/api/v1/profile/notifications", `{"news":true,"trades":true}`, false).Code)
	})

	t.Run("email off", func(t *testing.T) {
		s := newServer(t, false)

		require.Equal(t, nethttp.StatusServiceUnavailable,
			s.do("PUT", "/api/v1/profile/notifications", `{"news":true,"trades":true}`, true).Code)
	})
}

func Test_ConfirmEmail_NeedsNoCaller(t *testing.T) {
	s := newServer(t, true)
	at := time.Now()
	c := domain.Confirmation{
		TokenHash: domain.HashToken("tok"), PartyID: party, Email: "a@b.co", ExpiresAt: at.Add(time.Hour), ConfirmedAt: &at,
	}

	s.postgres.EXPECT().GetConfirmation(Any, c.TokenHash).Return(c, nil)
	s.postgres.EXPECT().GetProfileForUpdate(Any, party).Return(domain.Profile{PartyID: party, Email: "a@b.co"}, nil)

	w := s.do("POST", "/api/v1/email-confirmations", `{"token":"tok"}`, false)

	require.Equal(t, nethttp.StatusNoContent, w.Code, w.Body.String())
}

func Test_ConfirmEmail_ABadLinkIsGone(t *testing.T) {
	s := newServer(t, true)

	s.postgres.EXPECT().GetConfirmation(Any, domain.HashToken("tok")).Return(domain.Confirmation{}, domain.ErrNotFound)

	requireError(t, s.do("POST", "/api/v1/email-confirmations", `{"token":"tok"}`, false),
		nethttp.StatusGone, "This link is invalid or has expired.")
}

func Test_ConfirmEmail_RejectsAMissingToken(t *testing.T) {
	s := newServer(t, true)

	require.Equal(t, nethttp.StatusBadRequest, s.do("POST", "/api/v1/email-confirmations", `{}`, false).Code)
}

func signedWebhook(s server, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("POST", "/api/v1/webhooks/resend", strings.NewReader(body))
	r.Header.Set("svix-id", "msg_1")
	r.Header.Set("svix-timestamp", "1700000000")
	r.Header.Set("svix-signature", "v1,abc")

	w := httptest.NewRecorder()
	s.handler.ServeHTTP(w, r)

	return w
}

func Test_ResendWebhook_PassesTheRawBodyAndHeaders(t *testing.T) {
	s := newServer(t, true)
	body := `{"type": "contact.updated",   "data": {}}`

	s.contacts.EXPECT().ParseWebhook([]byte(body), "msg_1", "1700000000", "v1,abc").Return(domain.ContactEvent{}, nil)

	require.Equal(t, nethttp.StatusNoContent, signedWebhook(s, body).Code)
}

func Test_ResendWebhook_RejectsABadSignature(t *testing.T) {
	s := newServer(t, true)

	s.contacts.EXPECT().ParseWebhook(Any, Any, Any, Any).Return(domain.ContactEvent{}, domain.ErrUnauthorized)

	require.Equal(t, nethttp.StatusUnauthorized, signedWebhook(s, `{}`).Code)
}

func Test_ResendWebhook_RejectsAHugeBody(t *testing.T) {
	s := newServer(t, true)

	require.Equal(t, nethttp.StatusBadRequest, signedWebhook(s, strings.Repeat("x", 2<<20)).Code)
}
