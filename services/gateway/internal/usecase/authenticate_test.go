package usecase_test

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel"
	"github.com/vbloher/canton-private-otc-desk/pkg/principal"
	"github.com/vbloher/canton-private-otc-desk/services/gateway/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/gateway/internal/usecase"
	"github.com/vbloher/canton-private-otc-desk/services/gateway/internal/usecase/mocks"
)

const Any = mock.Anything

func alice() principal.Principal {
	return principal.Principal{UserID: uuid.New(), PartyID: "alice-3f9a1c::1220ab", Hint: "alice-3f9a1c"}
}

func Test_Authenticate_Cookie(t *testing.T) {
	otel.SilentModeInit()

	want := alice()

	identity := mocks.NewIdentity(t)
	identity.EXPECT().Authenticate(Any, "session-token").Return(want, nil)

	u := usecase.New(identity)

	actual, err := u.Authenticate(t.Context(), "session-token", "")
	require.NoError(t, err)
	require.Equal(t, want, actual)
}

func Test_Authenticate_APIKeyWins(t *testing.T) {
	otel.SilentModeInit()

	want := principal.Principal{UserID: uuid.New(), PartyID: "makerA::1220cd", Hint: "makerA"}

	// The mock fails the test if the session token is looked up as well.
	identity := mocks.NewIdentity(t)
	identity.EXPECT().AuthenticateAPIKey(Any, "demo-key-a").Return(want, nil)

	u := usecase.New(identity)

	actual, err := u.Authenticate(t.Context(), "session-token", "demo-key-a")
	require.NoError(t, err)
	require.Equal(t, want, actual)
}

func Test_Authenticate_None(t *testing.T) {
	otel.SilentModeInit()

	// No expectations: identity must not be called at all.
	u := usecase.New(mocks.NewIdentity(t))

	actual, err := u.Authenticate(t.Context(), "", "")
	require.ErrorIs(t, err, domain.ErrUnauthorized)
	require.Equal(t, principal.Principal{}, actual)
}

func Test_Authenticate_Rejected(t *testing.T) {
	otel.SilentModeInit()

	tests := map[string]struct {
		sessionToken string
		apiKey       string
		arrange      func(*mocks.Identity)
	}{
		"session token": {
			sessionToken: "expired",
			arrange: func(m *mocks.Identity) {
				m.EXPECT().Authenticate(Any, "expired").Return(principal.Principal{}, domain.ErrUnauthorized)
			},
		},
		"api key": {
			apiKey: "wrong",
			arrange: func(m *mocks.Identity) {
				m.EXPECT().AuthenticateAPIKey(Any, "wrong").Return(principal.Principal{}, domain.ErrUnauthorized)
			},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			identity := mocks.NewIdentity(t)
			tt.arrange(identity)

			u := usecase.New(identity)

			actual, err := u.Authenticate(t.Context(), tt.sessionToken, tt.apiKey)
			require.ErrorIs(t, err, domain.ErrUnauthorized)
			require.Equal(t, principal.Principal{}, actual)
		})
	}
}

func Test_Authenticate_IdentityFailure(t *testing.T) {
	otel.SilentModeInit()

	down := errors.New("identity is down")

	tests := map[string]struct {
		sessionToken string
		apiKey       string
		arrange      func(*mocks.Identity)
	}{
		"session token": {
			sessionToken: "token",
			arrange: func(m *mocks.Identity) {
				m.EXPECT().Authenticate(Any, "token").Return(principal.Principal{}, down)
			},
		},
		"api key": {
			apiKey: "key",
			arrange: func(m *mocks.Identity) {
				m.EXPECT().AuthenticateAPIKey(Any, "key").Return(principal.Principal{}, down)
			},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			identity := mocks.NewIdentity(t)
			tt.arrange(identity)

			u := usecase.New(identity)

			actual, err := u.Authenticate(t.Context(), tt.sessionToken, tt.apiKey)
			require.ErrorIs(t, err, down)
			require.NotErrorIs(t, err, domain.ErrUnauthorized, "an identity outage is not a bad credential")
			require.Equal(t, principal.Principal{}, actual)
		})
	}
}
