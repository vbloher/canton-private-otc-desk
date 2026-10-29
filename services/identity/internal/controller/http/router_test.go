package http_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	nethttp "net/http"

	"github.com/vbloher/canton-private-otc-desk/pkg/metrics"
	"github.com/vbloher/canton-private-otc-desk/pkg/otel"
	"github.com/vbloher/canton-private-otc-desk/pkg/router"
	controller "github.com/vbloher/canton-private-otc-desk/services/identity/internal/controller/http"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/usecase"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/usecase/mocks"
)

const (
	Any        = mock.Anything
	sessionTTL = 24 * time.Hour
)

var sharedMetrics = sync.OnceValue(metrics.NewHTTPServer)

type server struct {
	handler  nethttp.Handler
	postgres *mocks.Postgres
	redis    *mocks.Redis
	ledger   *mocks.Ledger
}

func newServer(t *testing.T, cookieSecure bool) server {
	t.Helper()

	otel.SilentModeInit()

	s := server{
		postgres: mocks.NewPostgres(t),
		redis:    mocks.NewRedis(t),
		ledger:   mocks.NewLedger(t),
	}

	uc := usecase.New(s.postgres, s.redis, s.ledger, usecase.Config{SessionTTL: sessionTTL})

	r := router.New()
	require.NoError(t, controller.IdentityRouter(r, uc, sharedMetrics(), controller.Config{CookieSecure: cookieSecure}))

	s.handler = r

	return s
}

type response struct {
	*httptest.ResponseRecorder
}

func (r response) json(t *testing.T) map[string]any {
	t.Helper()

	var body map[string]any

	require.NoError(t, json.Unmarshal(r.Body.Bytes(), &body), r.Body.String())

	return body
}

func (r response) jsonArray(t *testing.T) []map[string]any {
	t.Helper()

	var body []map[string]any

	require.NoError(t, json.Unmarshal(r.Body.Bytes(), &body), r.Body.String())

	return body
}

func (s server) do(t *testing.T, method, path, body string, header map[string]string) response {
	t.Helper()

	req := httptest.NewRequestWithContext(t.Context(), method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}

	for key, value := range header {
		req.Header.Set(key, value)
	}

	rec := httptest.NewRecorder()
	s.handler.ServeHTTP(rec, req)

	return response{rec}
}

func b64(b []byte) string {
	return base64.StdEncoding.EncodeToString(b)
}

func asUser(u domain.User) map[string]string {
	return map[string]string{
		"X-User-Id":    u.ID.String(),
		"X-Party-Id":   u.PartyID,
		"X-Party-Hint": u.Hint,
	}
}

func walletUser(t *testing.T) (domain.User, ed25519.PrivateKey) {
	t.Helper()

	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	user, err := domain.NewPasskeyUser(
		"alice-smith-3f9a1c::1220ab", "alice-smith-3f9a1c", "Alice Smith", publicKey, "cred-1", "blob-1",
	)
	require.NoError(t, err)

	return user, privateKey
}

func Test_Probes(t *testing.T) {
	s := newServer(t, false)

	for _, path := range []string{"/live", "/ready", "/metrics"} {
		require.Equal(t, nethttp.StatusOK, s.do(t, nethttp.MethodGet, path, "", nil).Code, path)
	}
}

func Test_RegisterStart_OK(t *testing.T) {
	s := newServer(t, false)

	publicKey := bytes.Repeat([]byte{1}, 32)
	topology := domain.Topology{
		PartyID: "alice-smith-3f9a1c::1220ab", Fingerprint: "1220ab", MultiHash: []byte{1, 2, 3, 250}, Transactions: []string{"tx"},
	}

	s.ledger.EXPECT().GenerateExternalParty(Any, Any, publicKey).Return(topology, nil)

	var parked domain.Registration

	s.redis.EXPECT().SaveRegistration(Any, Any, 10*time.Minute).
		Run(func(_ context.Context, r domain.Registration, _ time.Duration) { parked = r }).
		Return(nil)

	rec := s.do(t, nethttp.MethodPost, "/api/v1/auth/register/start",
		`{"displayName":"Alice Smith","publicKey":"`+b64(publicKey)+`"}`, nil)

	require.Equal(t, nethttp.StatusOK, rec.Code, rec.Body.String())
	require.Contains(t, rec.Header().Get("Content-Type"), "application/json")
	require.Equal(t, map[string]any{
		"registrationId": parked.ID.String(),
		"partyId":        "alice-smith-3f9a1c::1220ab",
		"multiHash":      b64([]byte{1, 2, 3, 250}),
	}, rec.json(t))
}

func Test_RegisterStart_InvalidInput(t *testing.T) {
	good := b64(bytes.Repeat([]byte{1}, 32))

	for name, body := range map[string]string{
		"not base64":                  `{"displayName":"Alice","publicKey":"` + strings.Repeat("!", 44) + `"}`,
		"31 byte key":                 `{"displayName":"Alice","publicKey":"` + b64(bytes.Repeat([]byte{1}, 31)) + `"}`,
		"key of the wrong text size":  `{"displayName":"Alice","publicKey":"AAAA"}`,
		"name too short once trimmed": `{"displayName":"  A ","publicKey":"` + good + `"}`,
		"name too short":              `{"displayName":"A","publicKey":"` + good + `"}`,
		"name too long":               `{"displayName":"` + strings.Repeat("x", 33) + `","publicKey":"` + good + `"}`,
		"missing key":                 `{"displayName":"Alice"}`,
		"unknown field":               `{"displayName":"Alice","publicKey":"` + good + `","admin":true}`,
		"not json":                    `{"displayName":`,
	} {
		s := newServer(t, false)

		rec := s.do(t, nethttp.MethodPost, "/api/v1/auth/register/start", body, nil)

		require.Equal(t, nethttp.StatusBadRequest, rec.Code, "%s: %s", name, rec.Body.String())
		require.NotEmpty(t, rec.json(t)["error"], name)
	}
}

func Test_RegisterStart_NamesTheBadField(t *testing.T) {
	s := newServer(t, false)

	rec := s.do(t, nethttp.MethodPost, "/api/v1/auth/register/start",
		`{"displayName":"Alice","publicKey":"`+strings.Repeat("!", 44)+`"}`, nil)

	require.Equal(t, nethttp.StatusBadRequest, rec.Code)
	require.Contains(t, rec.json(t)["error"], "publicKey")
}

func Test_RegisterStart_LedgerFailure(t *testing.T) {
	s := newServer(t, false)

	s.ledger.EXPECT().GenerateExternalParty(Any, Any, Any).
		Return(domain.Topology{}, failure(domain.ErrLedger, "rpc error: code = Unavailable desc = dial 10.0.0.7:9083"))

	rec := s.do(t, nethttp.MethodPost, "/api/v1/auth/register/start",
		`{"displayName":"Alice","publicKey":"`+b64(bytes.Repeat([]byte{1}, 32))+`"}`, nil)

	require.Equal(t, nethttp.StatusBadGateway, rec.Code, rec.Body.String())
	require.NotContains(t, rec.Body.String(), "10.0.0.7", "the ledger's address stays in the logs")
	require.NotEmpty(t, rec.json(t)["error"])
}

func parked(t *testing.T) (domain.Registration, []byte) {
	t.Helper()

	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	reg, err := domain.NewRegistration("Alice Smith", publicKey)
	require.NoError(t, err)

	reg.Topology = domain.Topology{
		PartyID: reg.Hint + "::1220ab", Fingerprint: "1220ab", MultiHash: bytes.Repeat([]byte{9}, 32), Transactions: []string{"tx"},
	}

	return reg, ed25519.Sign(privateKey, reg.Topology.MultiHash)
}

func completeBody(reg domain.Registration, signature []byte) string {
	return `{"registrationId":"` + reg.ID.String() + `","signature":"` + b64(signature) +
		`","credentialId":"cred-1","encryptedBlob":"blob-1"}`
}

var cookiePattern = regexp.MustCompile(`^otc_session=([A-Za-z0-9_-]{43}); Path=/; HttpOnly; SameSite=Lax; Max-Age=86400(; Secure)?$`)

func Test_RegisterComplete_CreatesWalletAndSetsCookie(t *testing.T) {
	for _, secure := range []bool{false, true} {
		s := newServer(t, secure)

		reg, signature := parked(t)

		var sessionHash string

		s.redis.EXPECT().TakeRegistration(Any, reg.ID).Return(reg, nil)
		s.postgres.EXPECT().GetUserByCredential(Any, "cred-1").Return(domain.User{}, domain.ErrNotFound)
		s.ledger.EXPECT().AllocateExternalParty(Any, reg.Topology, signature).Return(reg.Topology.PartyID, nil)
		s.postgres.EXPECT().CreateUser(Any, Any).Return(nil)
		s.postgres.EXPECT().CreateSession(Any, Any, Any, Any).
			Run(func(_ context.Context, hash string, _ uuid.UUID, _ time.Time) { sessionHash = hash }).
			Return(nil)

		rec := s.do(t, nethttp.MethodPost, "/api/v1/auth/register/complete", completeBody(reg, signature), nil)

		require.Equal(t, nethttp.StatusOK, rec.Code, rec.Body.String())

		body := rec.json(t)
		require.Equal(t, reg.Topology.PartyID, body["partyId"])
		require.Equal(t, reg.Hint, body["hint"])
		require.Equal(t, "Alice Smith", body["displayName"])
		require.Equal(t, "passkey", body["kind"])
		require.Equal(t, false, body["makerMode"])
		require.NotEmpty(t, body["id"])
		require.NotContains(t, body, "encryptedBlob")
		require.NotContains(t, body, "publicKey")

		header := rec.Header().Get("Set-Cookie")
		match := cookiePattern.FindStringSubmatch(header)
		require.NotNil(t, match, header)
		require.Equal(t, secure, match[2] == "; Secure", header)
		require.Equal(t, domain.HashSessionToken(match[1]), sessionHash, "the cookie holds the token behind the stored hash")

		cookies := rec.Result().Cookies()
		require.Len(t, cookies, 1)
		require.Equal(t, "otc_session", cookies[0].Name)
		require.True(t, cookies[0].HttpOnly)
		require.Equal(t, nethttp.SameSiteLaxMode, cookies[0].SameSite)
		require.Equal(t, "/", cookies[0].Path)
		require.Equal(t, int(sessionTTL.Seconds()), cookies[0].MaxAge)
		require.Equal(t, secure, cookies[0].Secure)
	}
}

func Test_RegisterComplete_Errors(t *testing.T) {
	reg, signature := parked(t)

	type setup func(s server)

	cases := map[string]struct {
		body   string
		setup  setup
		status int
	}{
		"registration expired": {
			body: completeBody(reg, signature),
			setup: func(s server) {
				s.redis.EXPECT().TakeRegistration(Any, reg.ID).Return(domain.Registration{}, domain.ErrRegistrationExpired)
			},
			status: nethttp.StatusGone,
		},
		"credential already registered": {
			body: completeBody(reg, signature),
			setup: func(s server) {
				s.redis.EXPECT().TakeRegistration(Any, reg.ID).Return(reg, nil)
				s.postgres.EXPECT().GetUserByCredential(Any, "cred-1").Return(domain.User{ID: uuid.New()}, nil)
			},
			status: nethttp.StatusConflict,
		},
		"ledger failure": {
			body: completeBody(reg, signature),
			setup: func(s server) {
				s.redis.EXPECT().TakeRegistration(Any, reg.ID).Return(reg, nil)
				s.postgres.EXPECT().GetUserByCredential(Any, "cred-1").Return(domain.User{}, domain.ErrNotFound)
				s.ledger.EXPECT().AllocateExternalParty(Any, Any, Any).Return("", failure(domain.ErrLedger, "boom"))
			},
			status: nethttp.StatusBadGateway,
		},
		"wrong signature": {
			body: completeBody(reg, bytes.Repeat([]byte{1}, 64)),
			setup: func(s server) {
				s.redis.EXPECT().TakeRegistration(Any, reg.ID).Return(reg, nil)
			},
			status: nethttp.StatusBadRequest,
		},
		"signature of the wrong size": {
			body:   completeBody(reg, signature[:63]),
			setup:  func(server) {},
			status: nethttp.StatusBadRequest,
		},
		"signature that is not base64": {
			body:   `{"registrationId":"` + reg.ID.String() + `","signature":"@@@","credentialId":"c","encryptedBlob":"b"}`,
			setup:  func(server) {},
			status: nethttp.StatusBadRequest,
		},
		"registration id that is not a uuid": {
			body:   `{"registrationId":"nope","signature":"` + b64(signature) + `","credentialId":"c","encryptedBlob":"b"}`,
			setup:  func(server) {},
			status: nethttp.StatusBadRequest,
		},
	}

	for name, c := range cases {
		s := newServer(t, false)
		c.setup(s)

		rec := s.do(t, nethttp.MethodPost, "/api/v1/auth/register/complete", c.body, nil)

		require.Equal(t, c.status, rec.Code, "%s: %s", name, rec.Body.String())
		require.NotEmpty(t, rec.json(t)["error"], name)
		require.Empty(t, rec.Header().Get("Set-Cookie"), "%s: no cookie without a session", name)
	}
}

func Test_RegisterComplete_ExpiredMessage(t *testing.T) {
	s := newServer(t, false)

	reg, signature := parked(t)

	s.redis.EXPECT().TakeRegistration(Any, reg.ID).Return(domain.Registration{}, domain.ErrRegistrationExpired)

	rec := s.do(t, nethttp.MethodPost, "/api/v1/auth/register/complete", completeBody(reg, signature), nil)

	require.Equal(t, map[string]any{"error": "request failed: registration expired"}, rec.json(t))
}

func Test_CreateChallenge_OK(t *testing.T) {
	s := newServer(t, false)

	user, _ := walletUser(t)

	s.postgres.EXPECT().GetUserByCredential(Any, "cred-1").Return(user, nil)

	var challenge string

	s.redis.EXPECT().SaveChallenge(Any, Any, user.ID, 5*time.Minute).
		Run(func(_ context.Context, c string, _ uuid.UUID, _ time.Duration) { challenge = c }).
		Return(nil)

	rec := s.do(t, nethttp.MethodPost, "/api/v1/auth/challenge", `{"credentialId":"cred-1"}`, nil)

	require.Equal(t, nethttp.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, map[string]any{
		"challenge":     challenge,
		"encryptedBlob": "blob-1",
		"publicKey":     b64(user.PublicKey),
	}, rec.json(t))
}

func Test_CreateChallenge_UnknownCredential(t *testing.T) {
	s := newServer(t, false)

	s.postgres.EXPECT().GetUserByCredential(Any, "nope").Return(domain.User{}, domain.ErrNotFound)

	rec := s.do(t, nethttp.MethodPost, "/api/v1/auth/challenge", `{"credentialId":"nope"}`, nil)

	require.Equal(t, nethttp.StatusNotFound, rec.Code, rec.Body.String())
	require.NotEmpty(t, rec.json(t)["error"])
}

func Test_CreateChallenge_EmptyCredential(t *testing.T) {
	s := newServer(t, false)

	rec := s.do(t, nethttp.MethodPost, "/api/v1/auth/challenge", `{"credentialId":""}`, nil)

	require.Equal(t, nethttp.StatusBadRequest, rec.Code)
}

func loginBody(challenge string, signature []byte) string {
	return `{"challenge":"` + challenge + `","signature":"` + b64(signature) + `","credentialId":"cred-1"}`
}

func Test_Login_OK(t *testing.T) {
	s := newServer(t, false)

	user, privateKey := walletUser(t)
	challenge := domain.NewChallenge()

	s.redis.EXPECT().TakeChallenge(Any, challenge).Return(user.ID, nil)
	s.postgres.EXPECT().GetUser(Any, user.ID).Return(user, nil)
	s.postgres.EXPECT().CreateSession(Any, Any, user.ID, Any).Return(nil)

	rec := s.do(t, nethttp.MethodPost, "/api/v1/auth/login",
		loginBody(challenge, ed25519.Sign(privateKey, []byte(challenge))), nil)

	require.Equal(t, nethttp.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, map[string]any{
		"id":          user.ID.String(),
		"partyId":     user.PartyID,
		"hint":        user.Hint,
		"displayName": "Alice Smith",
		"kind":        "passkey",
		"makerMode":   false,
	}, rec.json(t))
	require.Regexp(t, cookiePattern, rec.Header().Get("Set-Cookie"))
}

func Test_Login_BadSignature(t *testing.T) {
	s := newServer(t, false)

	user, _ := walletUser(t)
	challenge := domain.NewChallenge()

	_, otherKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	s.redis.EXPECT().TakeChallenge(Any, challenge).Return(user.ID, nil)
	s.postgres.EXPECT().GetUser(Any, user.ID).Return(user, nil)

	rec := s.do(t, nethttp.MethodPost, "/api/v1/auth/login",
		loginBody(challenge, ed25519.Sign(otherKey, []byte(challenge))), nil)

	require.Equal(t, nethttp.StatusUnauthorized, rec.Code, rec.Body.String())
	require.Empty(t, rec.Header().Get("Set-Cookie"))
	require.Equal(t, map[string]any{"error": "request failed: unauthorized"}, rec.json(t))
}

func Test_Login_UnknownChallenge(t *testing.T) {
	s := newServer(t, false)

	s.redis.EXPECT().TakeChallenge(Any, "otc-desk:nope").Return(uuid.Nil, domain.ErrUnauthorized)

	rec := s.do(t, nethttp.MethodPost, "/api/v1/auth/login", loginBody("otc-desk:nope", make([]byte, 64)), nil)

	require.Equal(t, nethttp.StatusUnauthorized, rec.Code)
}

func Test_Login_MalformedSignature(t *testing.T) {
	s := newServer(t, false)

	for name, signature := range map[string]string{
		"not base64":     "@@@",
		"wrong size":     b64([]byte("short")),
		"empty":          "",
		"oversize input": strings.Repeat("A", 200),
	} {
		rec := s.do(t, nethttp.MethodPost, "/api/v1/auth/login",
			`{"challenge":"otc-desk:x","signature":"`+signature+`","credentialId":"cred-1"}`, nil)

		require.Equal(t, nethttp.StatusBadRequest, rec.Code, "%s: %s", name, rec.Body.String())
	}
}

func Test_Logout_EndsTheSession(t *testing.T) {
	for _, secure := range []bool{false, true} {
		s := newServer(t, secure)

		hash := domain.HashSessionToken("the-token")

		s.postgres.EXPECT().DeleteSession(Any, hash).Return(nil)
		s.redis.EXPECT().DropCachedSession(Any, hash).Return()

		rec := s.do(t, nethttp.MethodPost, "/api/v1/auth/logout", "", map[string]string{"Cookie": "otc_session=the-token"})

		require.Equal(t, nethttp.StatusNoContent, rec.Code)
		require.Empty(t, rec.Body.String())

		expected := "otc_session=; Path=/; HttpOnly; SameSite=Lax; Max-Age=0"
		if secure {
			expected += "; Secure"
		}

		require.Equal(t, expected, rec.Header().Get("Set-Cookie"))
	}
}

func Test_Logout_WithoutACookieStillClearsIt(t *testing.T) {
	s := newServer(t, false)

	rec := s.do(t, nethttp.MethodPost, "/api/v1/auth/logout", "", nil)

	require.Equal(t, nethttp.StatusNoContent, rec.Code)
	require.Equal(t, "otc_session=; Path=/; HttpOnly; SameSite=Lax; Max-Age=0", rec.Header().Get("Set-Cookie"))
}

func Test_Logout_WithAnUnknownSessionStillSucceeds(t *testing.T) {
	s := newServer(t, false)

	hash := domain.HashSessionToken("stale")

	s.postgres.EXPECT().DeleteSession(Any, hash).Return(nil)
	s.redis.EXPECT().DropCachedSession(Any, hash).Return()

	rec := s.do(t, nethttp.MethodPost, "/api/v1/auth/logout", "", map[string]string{"Cookie": "otc_session=stale"})

	require.Equal(t, nethttp.StatusNoContent, rec.Code)
	require.Contains(t, rec.Header().Get("Set-Cookie"), "Max-Age=0")
}

func Test_Logout_StorageFailure(t *testing.T) {
	s := newServer(t, false)

	s.postgres.EXPECT().DeleteSession(Any, Any).Return(failure(nil, "connection reset"))

	rec := s.do(t, nethttp.MethodPost, "/api/v1/auth/logout", "", map[string]string{"Cookie": "otc_session=the-token"})

	require.Equal(t, nethttp.StatusInternalServerError, rec.Code)
	require.Empty(t, rec.Header().Get("Set-Cookie"), "the session is still alive, so the cookie stays")
	require.Equal(t, map[string]any{"error": "Internal Server Error"}, rec.json(t))
}

func Test_GetMe(t *testing.T) {
	s := newServer(t, false)

	user, _ := walletUser(t)

	s.postgres.EXPECT().GetUser(Any, user.ID).Return(user, nil)

	rec := s.do(t, nethttp.MethodGet, "/api/v1/me", "", asUser(user))

	require.Equal(t, nethttp.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, map[string]any{
		"id":          user.ID.String(),
		"partyId":     user.PartyID,
		"hint":        user.Hint,
		"displayName": "Alice Smith",
		"kind":        "passkey",
		"makerMode":   false,
	}, rec.json(t))
}

func Test_ProtectedRoutes_NeedACaller(t *testing.T) {
	s := newServer(t, false)

	user, _ := walletUser(t)

	incomplete := asUser(user)
	delete(incomplete, "X-Party-Hint")

	badID := asUser(user)
	badID["X-User-Id"] = "not-a-uuid"

	for _, header := range []map[string]string{nil, incomplete, badID} {
		for _, call := range []struct{ method, path, body string }{
			{nethttp.MethodGet, "/api/v1/me", ""},
			{nethttp.MethodPut, "/api/v1/me/maker-mode", `{"enabled":true}`},
			{nethttp.MethodGet, "/api/v1/makers", ""},
		} {
			rec := s.do(t, call.method, call.path, call.body, header)

			require.Equal(t, nethttp.StatusUnauthorized, rec.Code, "%s %s %v", call.method, call.path, header)
			require.Equal(t, map[string]any{"error": "request failed: unauthorized"}, rec.json(t))
		}
	}
}

func Test_GetMe_UserGone(t *testing.T) {
	s := newServer(t, false)

	user, _ := walletUser(t)

	s.postgres.EXPECT().GetUser(Any, user.ID).Return(domain.User{}, domain.ErrNotFound)

	rec := s.do(t, nethttp.MethodGet, "/api/v1/me", "", asUser(user))

	require.Equal(t, nethttp.StatusUnauthorized, rec.Code)
}

func Test_SetMakerMode(t *testing.T) {
	s := newServer(t, false)

	user, _ := walletUser(t)
	user.MakerMode = true

	s.postgres.EXPECT().SetMakerMode(Any, user.ID, true).Return(user, nil)

	rec := s.do(t, nethttp.MethodPut, "/api/v1/me/maker-mode", `{"enabled":true}`, asUser(user))

	require.Equal(t, nethttp.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, true, rec.json(t)["makerMode"])
}

func Test_SetMakerMode_InvalidBody(t *testing.T) {
	s := newServer(t, false)

	user, _ := walletUser(t)

	for name, body := range map[string]string{
		"missing":       `{}`,
		"not a boolean": `{"enabled":"yes"}`,
		"extra field":   `{"enabled":true,"kind":"house"}`,
	} {
		rec := s.do(t, nethttp.MethodPut, "/api/v1/me/maker-mode", body, asUser(user))

		require.Equal(t, nethttp.StatusBadRequest, rec.Code, "%s: %s", name, rec.Body.String())
	}
}

func Test_ListMakers(t *testing.T) {
	s := newServer(t, false)

	alice, _ := walletUser(t)
	makerA := domain.User{ID: uuid.New(), Hint: "makerA", DisplayName: "Maker A", PartyID: "makerA::1220ab"}
	makerB := domain.User{ID: uuid.New(), Hint: "makerB", DisplayName: "Maker B", PartyID: "makerB::1220ab"}

	s.postgres.EXPECT().ListMakers(Any).Return([]domain.User{makerA, alice, makerB}, nil)

	rec := s.do(t, nethttp.MethodGet, "/api/v1/makers", "", asUser(alice))

	require.Equal(t, nethttp.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, []map[string]any{
		{"hint": "makerA", "displayName": "Maker A"},
		{"hint": "makerB", "displayName": "Maker B"},
	}, rec.jsonArray(t), "the caller is left out, and only hints and names are shown")
}

func Test_ListMakers_NoneIsAnEmptyList(t *testing.T) {
	s := newServer(t, false)

	alice, _ := walletUser(t)

	s.postgres.EXPECT().ListMakers(Any).Return(nil, nil)

	rec := s.do(t, nethttp.MethodGet, "/api/v1/makers", "", asUser(alice))

	require.Equal(t, nethttp.StatusOK, rec.Code)
	require.JSONEq(t, `[]`, rec.Body.String())
}

func Test_UnknownRoute(t *testing.T) {
	s := newServer(t, false)

	require.Equal(t, nethttp.StatusNotFound, s.do(t, nethttp.MethodGet, "/api/v1/nope", "", nil).Code)
}
