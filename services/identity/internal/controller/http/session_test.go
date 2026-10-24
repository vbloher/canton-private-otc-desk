package http_test

import (
	"context"
	"crypto/ed25519"
	"maps"
	nethttp "net/http"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	identitypb "github.com/vbloher/canton-private-otc-desk/gen/grpc/identity_v1"
	grpcv1 "github.com/vbloher/canton-private-otc-desk/services/identity/internal/controller/grpc/v1"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/domain"
)

// session is the answer of register/complete and login.
type session struct {
	me        map[string]any
	token     string
	expiresAt time.Time
}

// sessionOf reads the Session answer: exactly the user, the token and the expiry, the expiry as
// UTC to the second (the one format every client parses).
func sessionOf(t *testing.T, rec response) session {
	t.Helper()

	body := rec.json(t)
	require.ElementsMatch(t, []string{"me", "sessionToken", "expiresAt"}, slices.Collect(maps.Keys(body)))

	me, ok := body["me"].(map[string]any)
	require.True(t, ok, "me is an object")

	token, ok := body["sessionToken"].(string)
	require.True(t, ok, "sessionToken is a string")
	require.NotEmpty(t, token)

	raw, ok := body["expiresAt"].(string)
	require.True(t, ok, "expiresAt is a string")
	require.Regexp(t, `^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z$`, raw)

	expiresAt, err := time.Parse(time.RFC3339, raw)
	require.NoError(t, err)

	return session{me: me, token: token, expiresAt: expiresAt}
}

// memorySessions stands in for the session tables of Postgres and for the cache in front of them,
// so that a token can be followed through sign-in, authentication and sign-out: a session exists
// once CreateSession stored it, ends when it is deleted or has expired, and the cache never hits.
type memorySessions struct {
	users    map[uuid.UUID]domain.User
	sessions map[string]storedSession // by token hash
}

type storedSession struct {
	userID    uuid.UUID
	expiresAt time.Time
}

func (s server) memorySessions(users ...domain.User) *memorySessions {
	m := &memorySessions{
		users:    map[uuid.UUID]domain.User{},
		sessions: map[string]storedSession{},
	}

	for _, user := range users {
		m.users[user.ID] = user
	}

	s.postgres.EXPECT().CreateSession(Any, Any, Any, Any).
		RunAndReturn(func(_ context.Context, hash string, userID uuid.UUID, expiresAt time.Time) error {
			m.sessions[hash] = storedSession{userID: userID, expiresAt: expiresAt}

			return nil
		}).Maybe()
	s.postgres.EXPECT().GetUserBySession(Any, Any, Any).
		RunAndReturn(func(_ context.Context, hash string, now time.Time) (domain.User, error) {
			stored, ok := m.sessions[hash]
			if !ok || !stored.expiresAt.After(now) {
				return domain.User{}, domain.ErrNotFound
			}

			return m.users[stored.userID], nil
		}).Maybe()
	s.postgres.EXPECT().DeleteSession(Any, Any).
		RunAndReturn(func(_ context.Context, hash string) error {
			delete(m.sessions, hash)

			return nil
		}).Maybe()
	s.redis.EXPECT().GetCachedSession(Any, Any).
		RunAndReturn(func(context.Context, string) (domain.User, bool) { return domain.User{}, false }).Maybe()
	s.redis.EXPECT().CacheSession(Any, Any, Any, Any).Return().Maybe()
	s.redis.EXPECT().DropCachedSession(Any, Any).Return().Maybe()

	return m
}

// authenticate asks the gRPC side, the way the gateway does, who a session token belongs to.
func (s server) authenticate(t *testing.T, token string) (*identitypb.User, error) {
	t.Helper()

	return grpcv1.New(s.usecase).Authenticate(t.Context(), &identitypb.AuthenticateInput{SessionToken: token})
}

func requireIsSignedIn(t *testing.T, s server, me map[string]any, token string) {
	t.Helper()

	user, err := s.authenticate(t, token)
	require.NoError(t, err)
	require.Equal(t, me["id"], user.GetId())
	require.Equal(t, me["partyId"], user.GetPartyId())
	require.Equal(t, me["hint"], user.GetHint())
	require.Equal(t, me["displayName"], user.GetDisplayName())
}

func requireNotSignedIn(t *testing.T, s server, token string) {
	t.Helper()

	_, err := s.authenticate(t, token)
	require.Equal(t, codes.Unauthenticated, status.Code(err), "%v", err)
}

// ---------------------------------------------------------------- the token in the answer

func Test_RegisterComplete_SessionTokenAuthenticatesOverGRPC(t *testing.T) {
	s := newServer(t, false)

	reg, signature := parked(t)
	sessions := s.memorySessions()

	s.redis.EXPECT().TakeRegistration(Any, reg.ID).Return(reg, nil)
	s.postgres.EXPECT().GetUserByCredential(Any, "cred-1").Return(domain.User{}, domain.ErrNotFound)
	s.ledger.EXPECT().AllocateExternalParty(Any, reg.Topology, signature).Return(reg.Topology.PartyID, nil)
	s.postgres.EXPECT().CreateUser(Any, Any).
		RunAndReturn(func(_ context.Context, user domain.User) error {
			sessions.users[user.ID] = user

			return nil
		})

	rec := s.do(t, nethttp.MethodPost, "/api/v1/auth/register/complete", completeBody(reg, signature), nil)
	require.Equal(t, nethttp.StatusOK, rec.Code, rec.Body.String())

	got := sessionOf(t, rec)
	require.Equal(t, reg.Topology.PartyID, got.me["partyId"])

	requireIsSignedIn(t, s, got.me, got.token)
	requireNotSignedIn(t, s, got.token+"x")

	require.True(t, got.expiresAt.After(time.Now().Add(sessionTTL-time.Minute)), "about a session lifetime from now")
	require.True(t, got.expiresAt.Before(time.Now().Add(sessionTTL+time.Minute)))
}

func Test_Login_SessionTokenAuthenticatesOverGRPC(t *testing.T) {
	s := newServer(t, false)

	user, privateKey := walletUser(t)
	challenge := domain.NewChallenge()

	s.memorySessions(user)
	s.redis.EXPECT().TakeChallenge(Any, challenge).Return(user.ID, nil)
	s.postgres.EXPECT().GetUser(Any, user.ID).Return(user, nil)

	rec := s.do(t, nethttp.MethodPost, "/api/v1/auth/login",
		loginBody(challenge, ed25519.Sign(privateKey, []byte(challenge))), nil)
	require.Equal(t, nethttp.StatusOK, rec.Code, rec.Body.String())

	got := sessionOf(t, rec)

	requireIsSignedIn(t, s, got.me, got.token)
	requireNotSignedIn(t, s, "not-"+got.token)

	require.True(t, got.expiresAt.After(time.Now().Add(sessionTTL-time.Minute)), "about a session lifetime from now")
	require.True(t, got.expiresAt.Before(time.Now().Add(sessionTTL+time.Minute)))
}

// ---------------------------------------------------------------- POST /auth/logout by bearer token

const clearedCookie = "otc_session=; Path=/; HttpOnly; SameSite=Lax; Max-Age=0"

func Test_Logout_ByBearerToken(t *testing.T) {
	s := newServer(t, false)

	hash := domain.HashSessionToken("the-token")

	s.postgres.EXPECT().DeleteSession(Any, hash).Return(nil)
	s.redis.EXPECT().DropCachedSession(Any, hash).Return()

	rec := s.do(t, nethttp.MethodPost, "/api/v1/auth/logout", "", map[string]string{"Authorization": "Bearer the-token"})

	require.Equal(t, nethttp.StatusNoContent, rec.Code)
	require.Empty(t, rec.Body.String())
	require.Equal(t, clearedCookie, rec.Header().Get("Set-Cookie"))
}

func Test_Logout_ByBearerToken_SchemeIsCaseInsensitive(t *testing.T) {
	for _, scheme := range []string{"bearer", "BEARER", "BeArEr"} {
		s := newServer(t, false)

		hash := domain.HashSessionToken("the-token")

		s.postgres.EXPECT().DeleteSession(Any, hash).Return(nil)
		s.redis.EXPECT().DropCachedSession(Any, hash).Return()

		rec := s.do(t, nethttp.MethodPost, "/api/v1/auth/logout", "", map[string]string{"Authorization": scheme + " the-token"})

		require.Equal(t, nethttp.StatusNoContent, rec.Code, scheme)
	}
}

// The gateway authenticates with the bearer token before the cookie, so a client that sends both
// may be using either one. Logout ends the session of each.
func Test_Logout_EndsTheCookieSessionAndTheBearerSessionWhenTheyDiffer(t *testing.T) {
	s := newServer(t, false)

	cookieHash := domain.HashSessionToken("cookie-token")
	bearerHash := domain.HashSessionToken("bearer-token")

	s.postgres.EXPECT().DeleteSession(Any, cookieHash).Return(nil).Once()
	s.postgres.EXPECT().DeleteSession(Any, bearerHash).Return(nil).Once()
	s.redis.EXPECT().DropCachedSession(Any, cookieHash).Return().Once()
	s.redis.EXPECT().DropCachedSession(Any, bearerHash).Return().Once()

	rec := s.do(t, nethttp.MethodPost, "/api/v1/auth/logout", "", map[string]string{
		"Cookie":        "otc_session=cookie-token",
		"Authorization": "Bearer bearer-token",
	})

	require.Equal(t, nethttp.StatusNoContent, rec.Code)
	require.Equal(t, clearedCookie, rec.Header().Get("Set-Cookie"))
}

// Naming one session twice ends it once: the second delete or cache drop would be an unexpected call.
func Test_Logout_EndsASessionOnceWhenTheCookieAndTheBearerTokenAreTheSame(t *testing.T) {
	s := newServer(t, false)

	hash := domain.HashSessionToken("the-token")

	s.postgres.EXPECT().DeleteSession(Any, hash).Return(nil).Once()
	s.redis.EXPECT().DropCachedSession(Any, hash).Return().Once()

	rec := s.do(t, nethttp.MethodPost, "/api/v1/auth/logout", "", map[string]string{
		"Cookie":        "otc_session=the-token",
		"Authorization": "Bearer the-token",
	})

	require.Equal(t, nethttp.StatusNoContent, rec.Code)
	require.Equal(t, clearedCookie, rec.Header().Get("Set-Cookie"))
}

// The same through the session stand-in: the two sessions the request names are gone for the
// gateway's Authenticate, a third session of the same user is not touched.
func Test_Logout_EndsBothSessionsAClientPresents(t *testing.T) {
	s := newServer(t, false)

	user, _ := walletUser(t)
	sessions := s.memorySessions(user)

	for _, token := range []string{"cookie-token", "bearer-token", "other-token"} {
		sessions.sessions[domain.HashSessionToken(token)] = storedSession{
			userID:    user.ID,
			expiresAt: time.Now().Add(time.Hour),
		}
	}

	me := map[string]any{
		"id": user.ID.String(), "partyId": user.PartyID, "hint": user.Hint, "displayName": user.DisplayName,
	}

	for _, token := range []string{"cookie-token", "bearer-token", "other-token"} {
		requireIsSignedIn(t, s, me, token)
	}

	rec := s.do(t, nethttp.MethodPost, "/api/v1/auth/logout", "", map[string]string{
		"Cookie":        "otc_session=cookie-token",
		"Authorization": "Bearer bearer-token",
	})
	require.Equal(t, nethttp.StatusNoContent, rec.Code)

	requireNotSignedIn(t, s, "cookie-token")
	requireNotSignedIn(t, s, "bearer-token")
	requireIsSignedIn(t, s, me, "other-token")

	again := s.do(t, nethttp.MethodPost, "/api/v1/auth/logout", "", map[string]string{
		"Cookie":        "otc_session=cookie-token",
		"Authorization": "Bearer bearer-token",
	})
	require.Equal(t, nethttp.StatusNoContent, again.Code, "signing out twice is fine")
}

// A cookie without a value signs nobody in, so it does not hide a bearer token.
func Test_Logout_EmptyCookieDoesNotHideTheBearerToken(t *testing.T) {
	s := newServer(t, false)

	hash := domain.HashSessionToken("bearer-token")

	s.postgres.EXPECT().DeleteSession(Any, hash).Return(nil)
	s.redis.EXPECT().DropCachedSession(Any, hash).Return()

	rec := s.do(t, nethttp.MethodPost, "/api/v1/auth/logout", "", map[string]string{
		"Cookie":        "otc_session=",
		"Authorization": "Bearer bearer-token",
	})

	require.Equal(t, nethttp.StatusNoContent, rec.Code)
}

// An unusable Authorization header is no token: the cookie's session still ends, and nothing else.
func Test_Logout_UnusableAuthorizationDoesNotHideTheCookie(t *testing.T) {
	for _, header := range []string{"Bearer", "Basic dXNlcjpwYXNz", "Bearer one two", "garbage"} {
		s := newServer(t, false)

		hash := domain.HashSessionToken("cookie-token")

		s.postgres.EXPECT().DeleteSession(Any, hash).Return(nil).Once()
		s.redis.EXPECT().DropCachedSession(Any, hash).Return().Once()

		rec := s.do(t, nethttp.MethodPost, "/api/v1/auth/logout", "", map[string]string{
			"Cookie":        "otc_session=cookie-token",
			"Authorization": header,
		})

		require.Equal(t, nethttp.StatusNoContent, rec.Code, header)
		require.Equal(t, clearedCookie, rec.Header().Get("Set-Cookie"), header)
	}
}

// Anything but one "Bearer <token>" is no token, and logout is then what it is without a cookie:
// 204, the cookie cleared, nothing touched.
func Test_Logout_UnusableAuthorizationIsNoToken(t *testing.T) {
	for name, header := range map[string]string{
		"scheme only":               "Bearer",
		"scheme and a space":        "Bearer ",
		"another scheme":            "Basic dXNlcjpwYXNz",
		"a token without a scheme":  "the-token",
		"two tokens":                "Bearer one two",
		"a tab instead of a space":  "Bearer\tthe-token",
		"the scheme glued to token": "Bearerthe-token",
		"nothing":                   "",
	} {
		s := newServer(t, false) // no expectations: nothing may be looked up or deleted

		rec := s.do(t, nethttp.MethodPost, "/api/v1/auth/logout", "", map[string]string{"Authorization": header})

		require.Equal(t, nethttp.StatusNoContent, rec.Code, name)
		require.Equal(t, clearedCookie, rec.Header().Get("Set-Cookie"), name)
	}
}

func Test_Logout_ByBearerToken_StorageFailure(t *testing.T) {
	s := newServer(t, false)

	s.postgres.EXPECT().DeleteSession(Any, Any).Return(failure(nil, "connection reset"))

	rec := s.do(t, nethttp.MethodPost, "/api/v1/auth/logout", "", map[string]string{"Authorization": "Bearer the-token"})

	require.Equal(t, nethttp.StatusInternalServerError, rec.Code)
	require.Empty(t, rec.Header().Get("Set-Cookie"), "the session is still alive, so the cookie stays")
}

// The whole round trip the SPA on another origin makes: sign in, be known to the gateway's
// Authenticate, sign out with the bearer token, be unknown.
func Test_Logout_ByBearerToken_EndsTheSession(t *testing.T) {
	s := newServer(t, false)

	user, privateKey := walletUser(t)
	challenge := domain.NewChallenge()

	s.memorySessions(user)
	s.redis.EXPECT().TakeChallenge(Any, challenge).Return(user.ID, nil)
	s.postgres.EXPECT().GetUser(Any, user.ID).Return(user, nil)

	login := s.do(t, nethttp.MethodPost, "/api/v1/auth/login",
		loginBody(challenge, ed25519.Sign(privateKey, []byte(challenge))), nil)
	require.Equal(t, nethttp.StatusOK, login.Code, login.Body.String())

	got := sessionOf(t, login)
	requireIsSignedIn(t, s, got.me, got.token)

	out := s.do(t, nethttp.MethodPost, "/api/v1/auth/logout", "", map[string]string{"Authorization": "Bearer " + got.token})
	require.Equal(t, nethttp.StatusNoContent, out.Code)

	requireNotSignedIn(t, s, got.token)

	again := s.do(t, nethttp.MethodPost, "/api/v1/auth/logout", "", map[string]string{"Authorization": "Bearer " + got.token})
	require.Equal(t, nethttp.StatusNoContent, again.Code, "signing out twice is fine")
}
