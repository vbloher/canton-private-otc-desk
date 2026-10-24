package usecase_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/dto"
)

func Test_Logout_DeletesTheSessionAndItsCacheEntry(t *testing.T) {
	d := newDeps(t)

	hash := domain.HashSessionToken("token")

	d.postgres.EXPECT().DeleteSession(Any, hash).Return(nil)
	d.redis.EXPECT().DropCachedSession(Any, hash).Return()

	require.NoError(t, d.useCase().Logout(t.Context(), dto.LogoutInput{Tokens: []string{"token"}}))
}

// A client can present a cookie and a bearer token that name different sessions, and the gateway
// prefers the bearer token: whichever it used must end, so both do.
func Test_Logout_EndsEverySessionTheRequestNames(t *testing.T) {
	d := newDeps(t)

	for _, token := range []string{"cookie-token", "bearer-token"} {
		hash := domain.HashSessionToken(token)

		d.postgres.EXPECT().DeleteSession(Any, hash).Return(nil).Once()
		d.redis.EXPECT().DropCachedSession(Any, hash).Return().Once()
	}

	require.NoError(t, d.useCase().Logout(t.Context(), dto.LogoutInput{Tokens: []string{"cookie-token", "bearer-token"}}))
}

func Test_Logout_NamingTheSameSessionTwiceEndsItOnce(t *testing.T) {
	d := newDeps(t)

	hash := domain.HashSessionToken("token")

	// .Once(): a second delete or a second cache drop would be an unexpected call.
	d.postgres.EXPECT().DeleteSession(Any, hash).Return(nil).Once()
	d.redis.EXPECT().DropCachedSession(Any, hash).Return().Once()

	require.NoError(t, d.useCase().Logout(t.Context(), dto.LogoutInput{Tokens: []string{"token", "token"}}))
}

func Test_Logout_WithoutATokenDoesNothing(t *testing.T) {
	for name, tokens := range map[string][]string{
		"no tokens":         nil,
		"an empty token":    {""},
		"two empty tokens":  {"", ""},
		"an empty slice":    {},
		"nothing but empty": {"", "", ""},
	} {
		t.Run(name, func(t *testing.T) {
			d := newDeps(t) // no expectations

			require.NoError(t, d.useCase().Logout(t.Context(), dto.LogoutInput{Tokens: tokens}))
		})
	}
}

// A cookie without a value is no session; it does not stand in the way of the other token.
func Test_Logout_IgnoresAnEmptyToken(t *testing.T) {
	d := newDeps(t)

	hash := domain.HashSessionToken("token")

	d.postgres.EXPECT().DeleteSession(Any, hash).Return(nil)
	d.redis.EXPECT().DropCachedSession(Any, hash).Return()

	require.NoError(t, d.useCase().Logout(t.Context(), dto.LogoutInput{Tokens: []string{"", "token"}}))
}

func Test_Logout_KeepsTheCacheWhenTheDeleteFails(t *testing.T) {
	d := newDeps(t)

	boom := errors.New("connection reset")

	d.postgres.EXPECT().DeleteSession(Any, domain.HashSessionToken("token")).Return(boom)
	// No DropCachedSession expectation: the session is still valid, so the cache keeps it.

	err := d.useCase().Logout(t.Context(), dto.LogoutInput{Tokens: []string{"token"}})
	require.ErrorIs(t, err, boom)
}

// One session that cannot be ended does not spare the other: that one is ended and its cache entry
// dropped, and the error is returned so the caller can say so (and the client can try again).
func Test_Logout_AFailedSessionDoesNotSpareTheOthers(t *testing.T) {
	d := newDeps(t)

	boom := errors.New("connection reset")
	firstHash := domain.HashSessionToken("first")
	secondHash := domain.HashSessionToken("second")

	d.postgres.EXPECT().DeleteSession(Any, firstHash).Return(boom)
	d.postgres.EXPECT().DeleteSession(Any, secondHash).Return(nil)
	d.redis.EXPECT().DropCachedSession(Any, secondHash).Return()
	// No DropCachedSession for the first: its session is still valid.

	err := d.useCase().Logout(t.Context(), dto.LogoutInput{Tokens: []string{"first", "second"}})
	require.ErrorIs(t, err, boom)
}
