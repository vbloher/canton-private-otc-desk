//go:build integration

// An end-to-end run of the whole service against the Postgres and Redis of `make up`, with a fake
// ledger standing in for the ledger service. It starts the service before the ledger is up, as
// docker compose may, and walks a wallet through registration, sign-in and sign-out.
package app_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	identitypb "github.com/vbloher/canton-private-otc-desk/gen/grpc/identity_v1"
	ledgerpb "github.com/vbloher/canton-private-otc-desk/gen/grpc/ledger_v1"
	"github.com/vbloher/canton-private-otc-desk/pkg/grpcclient"
	"github.com/vbloher/canton-private-otc-desk/pkg/grpcserver"
	"github.com/vbloher/canton-private-otc-desk/pkg/httpserver"
	"github.com/vbloher/canton-private-otc-desk/pkg/otel"
	"github.com/vbloher/canton-private-otc-desk/pkg/postgres"
	"github.com/vbloher/canton-private-otc-desk/pkg/redis"
	"github.com/vbloher/canton-private-otc-desk/services/identity/config"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/adapter/ledger"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/app"
	httpcontroller "github.com/vbloher/canton-private-otc-desk/services/identity/internal/controller/http"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/usecase"
)

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}

	return fallback
}

// newDatabase creates a throwaway database for one run and drops it afterwards.
func newDatabase(t *testing.T) postgres.Config {
	t.Helper()

	cfg := postgres.Config{
		User:     getenv("POSTGRES_USER", "otc"),
		Password: getenv("POSTGRES_PASSWORD", "otc"),
		Port:     getenv("POSTGRES_PORT", "5432"),
		Host:     getenv("POSTGRES_HOST", "localhost"),
		DBName:   "postgres",
	}

	admin, err := pgx.Connect(t.Context(), cfg.URL())
	require.NoError(t, err)

	suffix := make([]byte, 6)
	_, _ = rand.Read(suffix)
	name := "identity_e2e_" + hex.EncodeToString(suffix)

	_, err = admin.Exec(t.Context(), "CREATE DATABASE "+name)
	require.NoError(t, err)

	t.Cleanup(func() {
		ctx := context.Background()

		_, err := admin.Exec(ctx, "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)")
		require.NoError(t, err)
		require.NoError(t, admin.Close(ctx))
	})

	cfg.DBName = name

	return cfg
}

func freeAddr(t *testing.T) string {
	t.Helper()

	var lc net.ListenConfig

	l, err := lc.Listen(t.Context(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	require.NoError(t, l.Close())

	return l.Addr().String()
}

func port(addr string) string {
	_, p, _ := net.SplitHostPort(addr)

	return p
}

// fakeLedger knows the house parties and onboards external parties the way Canton does: it
// only allocates a party when the key signed the onboarding hash.
type fakeLedger struct {
	ledgerpb.UnimplementedLedgerV1Server

	mu        sync.Mutex
	pending   map[string]pendingParty // by topology transaction
	allocated []string
}

type pendingParty struct {
	partyID   string
	publicKey []byte
	multiHash []byte
}

func (f *fakeLedger) ResolveParty(_ context.Context, in *ledgerpb.ResolvePartyInput) (*ledgerpb.ResolvePartyOutput, error) {
	switch in.GetHint() {
	case "makerA", "makerB":
		return &ledgerpb.ResolvePartyOutput{PartyId: in.GetHint() + "::1220feed"}, nil
	default:
		return nil, status.Error(codes.NotFound, "no such party")
	}
}

func (f *fakeLedger) GenerateExternalParty(
	_ context.Context, in *ledgerpb.GenerateExternalPartyInput,
) (*ledgerpb.GenerateExternalPartyOutput, error) {
	sum := sha256.Sum256(in.GetPublicKey())
	fingerprint := "1220" + hex.EncodeToString(sum[:8])
	partyID := in.GetHint() + "::" + fingerprint
	multiHash := sha256.Sum256([]byte("onboard:" + partyID))
	transaction := "tx:" + partyID

	f.mu.Lock()
	defer f.mu.Unlock()

	if f.pending == nil {
		f.pending = map[string]pendingParty{}
	}

	f.pending[transaction] = pendingParty{partyID: partyID, publicKey: in.GetPublicKey(), multiHash: multiHash[:]}

	return &ledgerpb.GenerateExternalPartyOutput{
		PartyId:              partyID,
		Fingerprint:          fingerprint,
		MultiHash:            multiHash[:],
		TopologyTransactions: []string{transaction},
	}, nil
}

func (f *fakeLedger) AllocateExternalParty(
	_ context.Context, in *ledgerpb.AllocateExternalPartyInput,
) (*ledgerpb.AllocateExternalPartyOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	party, ok := f.pending[in.GetTopologyTransactions()[0]]
	if !ok || !ed25519.Verify(party.publicKey, party.multiHash, in.GetSignature()) {
		return nil, status.Error(codes.InvalidArgument, "topology is not authorised")
	}

	f.allocated = append(f.allocated, party.partyID)

	return &ledgerpb.AllocateExternalPartyOutput{PartyId: party.partyID}, nil
}

func (f *fakeLedger) allocations() []string {
	f.mu.Lock()
	defer f.mu.Unlock()

	return append([]string(nil), f.allocated...)
}

func startLedger(t *testing.T, addr string) *fakeLedger {
	t.Helper()

	var lc net.ListenConfig

	l, err := lc.Listen(t.Context(), "tcp", addr)
	require.NoError(t, err)

	fake := &fakeLedger{}
	s := grpc.NewServer()
	ledgerpb.RegisterLedgerV1Server(s, fake)

	go func() { _ = s.Serve(l) }()

	t.Cleanup(s.Stop)

	return fake
}

type client struct {
	t    *testing.T
	base string
}

type reply struct {
	status int
	header http.Header
	body   []byte
}

func (r reply) json(t *testing.T) map[string]any {
	t.Helper()

	var body map[string]any

	require.NoError(t, json.Unmarshal(r.body, &body), string(r.body))

	return body
}

func (c client) do(method, path, body string, header map[string]string) reply {
	c.t.Helper()

	req, err := http.NewRequestWithContext(c.t.Context(), method, c.base+path, strings.NewReader(body))
	require.NoError(c.t, err)

	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}

	for key, value := range header {
		req.Header.Set(key, value)
	}

	res, err := http.DefaultClient.Do(req)
	require.NoError(c.t, err)

	defer res.Body.Close()

	data, err := io.ReadAll(res.Body)
	require.NoError(c.t, err)

	return reply{status: res.StatusCode, header: res.Header, body: data}
}

func b64(b []byte) string { return base64.StdEncoding.EncodeToString(b) }

func cookieOf(t *testing.T, r reply) string {
	t.Helper()

	res := http.Response{Header: r.header}
	cookies := res.Cookies()
	require.Len(t, cookies, 1, r.header.Get("Set-Cookie"))
	require.Equal(t, "otc_session", cookies[0].Name)

	return cookies[0].Value
}

func asCaller(me map[string]any) map[string]string {
	return map[string]string{
		"X-User-Id":    me["id"].(string),
		"X-Party-Id":   me["partyId"].(string),
		"X-Party-Hint": me["hint"].(string),
	}
}

// app.Run registers the HTTP collectors of pkg/metrics on the global Prometheus registry, so the
// service can start once per process.
var started atomic.Bool

func Test_Service_EndToEnd(t *testing.T) {
	if !started.CompareAndSwap(false, true) {
		t.Skip("app.Run can start once per process (global Prometheus collectors): run this test with -count=1")
	}

	otel.SilentModeInit()

	httpAddr, grpcAddr, ledgerAddr := freeAddr(t), freeAddr(t), freeAddr(t)

	cfg := config.Config{
		HTTP:     httpserver.Config{Port: port(httpAddr)},
		GRPC:     grpcserver.Config{Port: port(grpcAddr)},
		Router:   httpcontroller.Config{CookieSecure: false},
		Postgres: newDatabase(t),
		Redis:    redis.Config{Addr: getenv("REDIS_ADDR", "localhost:6379")},
		Ledger:   ledger.Config{Addr: ledgerAddr},
		UseCase: usecase.Config{
			SessionTTL: time.Hour,
			HouseMakers: domain.HouseMakers{
				{Hint: "makerA", APIKey: "demo-key-a"},
				{Hint: "makerB", APIKey: "demo-key-b"},
			},
		},
	}

	ctx, stop := context.WithCancel(t.Context())
	defer stop()

	done := make(chan error, 1)

	go func() { done <- app.Run(ctx, cfg) }()

	api := client{t: t, base: "http://" + httpAddr}

	// The ledger is not up: the service answers anyway.
	require.Eventually(t, func() bool {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, api.base+"/live", nil)
		if err != nil {
			return false
		}

		res, err := http.DefaultClient.Do(req)
		if err != nil {
			return false
		}

		_ = res.Body.Close()

		return res.StatusCode == http.StatusOK
	}, 10*time.Second, 50*time.Millisecond, "the service must come up without the ledger")

	conn, err := grpcclient.New(grpcAddr, 5*time.Second)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	identity := identitypb.NewIdentityV1Client(conn)

	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	startBody := `{"displayName":"Alice Smith","publicKey":"` + b64(publicKey) + `"}`

	down := api.do(http.MethodPost, "/api/v1/auth/register/start", startBody, nil)
	require.Equal(t, http.StatusBadGateway, down.status, string(down.body))

	_, err = identity.AuthenticateAPIKey(ctx, &identitypb.AuthenticateAPIKeyInput{ApiKey: "demo-key-a"})
	require.Equal(t, codes.Unauthenticated, status.Code(err), "the house makers are not seeded yet")

	// The ledger comes up; the house makers are seeded by the retry loop.
	fake := startLedger(t, ledgerAddr)

	require.Eventually(t, func() bool {
		user, err := identity.AuthenticateAPIKey(ctx, &identitypb.AuthenticateAPIKeyInput{ApiKey: "demo-key-a"})

		return err == nil && user.GetHint() == "makerA"
	}, 15*time.Second, 100*time.Millisecond, "the house makers must be seeded once the ledger answers")

	makerA, err := identity.AuthenticateAPIKey(ctx, &identitypb.AuthenticateAPIKeyInput{ApiKey: "demo-key-a"})
	require.NoError(t, err)
	require.Equal(t, "makerA::1220feed", makerA.GetPartyId())
	require.Equal(t, "makerA", makerA.GetDisplayName())
	require.True(t, makerA.GetMakerMode())

	_, err = identity.AuthenticateAPIKey(ctx, &identitypb.AuthenticateAPIKeyInput{ApiKey: "demo-key-z"})
	require.Equal(t, codes.Unauthenticated, status.Code(err))

	// Registration: start, sign the onboarding hash with the browser key, complete.
	start := api.do(http.MethodPost, "/api/v1/auth/register/start", startBody, nil)
	require.Equal(t, http.StatusOK, start.status, string(start.body))

	multiHash, err := base64.StdEncoding.DecodeString(start.json(t)["multiHash"].(string))
	require.NoError(t, err)

	completeBody := `{"registrationId":"` + start.json(t)["registrationId"].(string) + `","signature":"` +
		b64(ed25519.Sign(privateKey, multiHash)) + `","credentialId":"cred-e2e","encryptedBlob":"blob-e2e"}`

	complete := api.do(http.MethodPost, "/api/v1/auth/register/complete", completeBody, nil)
	require.Equal(t, http.StatusOK, complete.status, string(complete.body))

	me := complete.json(t)
	require.Equal(t, start.json(t)["partyId"], me["partyId"])
	require.Regexp(t, `^alice-smith-[0-9a-f]{6}$`, me["hint"])
	require.Equal(t, "passkey", me["kind"])
	require.Equal(t, false, me["makerMode"])

	setCookie := complete.header.Get("Set-Cookie")
	require.Contains(t, setCookie, "; Path=/; HttpOnly; SameSite=Lax; Max-Age=3600")
	require.NotContains(t, setCookie, "Secure")

	token := cookieOf(t, complete)

	// The registration completes once; the same passkey cannot back a second wallet.
	replay := api.do(http.MethodPost, "/api/v1/auth/register/complete", completeBody, nil)
	require.Equal(t, http.StatusGone, replay.status, string(replay.body))

	second := api.do(http.MethodPost, "/api/v1/auth/register/start", startBody, nil)
	require.Equal(t, http.StatusOK, second.status)

	secondHash, err := base64.StdEncoding.DecodeString(second.json(t)["multiHash"].(string))
	require.NoError(t, err)

	dup := api.do(http.MethodPost, "/api/v1/auth/register/complete",
		`{"registrationId":"`+second.json(t)["registrationId"].(string)+`","signature":"`+
			b64(ed25519.Sign(privateKey, secondHash))+`","credentialId":"cred-e2e","encryptedBlob":"blob"}`, nil)
	require.Equal(t, http.StatusConflict, dup.status, string(dup.body))
	require.Len(t, fake.allocations(), 1, "the clash is found before a second party is allocated")

	// The gateway's view of the session: gRPC Authenticate, twice (the second one is a cache hit).
	for range 2 {
		user, err := identity.Authenticate(ctx, &identitypb.AuthenticateInput{SessionToken: token})
		require.NoError(t, err)
		require.Equal(t, me["id"], user.GetId())
		require.Equal(t, me["partyId"], user.GetPartyId())
		require.Equal(t, me["hint"], user.GetHint())
		require.Equal(t, "Alice Smith", user.GetDisplayName())
	}

	// The routes behind the gateway, which sends the caller as headers.
	caller := asCaller(me)

	got := api.do(http.MethodGet, "/api/v1/me", "", caller)
	require.Equal(t, http.StatusOK, got.status, string(got.body))
	require.Equal(t, me, got.json(t))

	require.Equal(t, http.StatusUnauthorized, api.do(http.MethodGet, "/api/v1/me", "", nil).status)

	var makers []map[string]string

	list := api.do(http.MethodGet, "/api/v1/makers", "", caller)
	require.NoError(t, json.Unmarshal(list.body, &makers))
	require.Equal(t, []map[string]string{
		{"hint": "makerA", "displayName": "makerA"},
		{"hint": "makerB", "displayName": "makerB"},
	}, makers)

	toggled := api.do(http.MethodPut, "/api/v1/me/maker-mode", `{"enabled":true}`, caller)
	require.Equal(t, http.StatusOK, toggled.status, string(toggled.body))
	require.Equal(t, true, toggled.json(t)["makerMode"])

	resolved, err := identity.ResolveMakers(ctx, &identitypb.ResolveMakersInput{
		Hints: []string{me["hint"].(string), "makerB", "nobody"},
	})
	require.NoError(t, err)
	require.Len(t, resolved.GetMakers(), 2)
	require.Equal(t, me["hint"], resolved.GetMakers()[0].GetHint(), "in request order, now that maker mode is on")
	require.Equal(t, "makerB", resolved.GetMakers()[1].GetHint())

	list = api.do(http.MethodGet, "/api/v1/makers", "", caller)
	require.NoError(t, json.Unmarshal(list.body, &makers))
	require.Len(t, makers, 2, "the caller is never in their own list")

	// Sign-in: challenge, sign it with the key, login. A challenge works once.
	challenge := api.do(http.MethodPost, "/api/v1/auth/challenge", `{"credentialId":"cred-e2e"}`, nil)
	require.Equal(t, http.StatusOK, challenge.status, string(challenge.body))
	require.Equal(t, "blob-e2e", challenge.json(t)["encryptedBlob"])
	require.Equal(t, b64(publicKey), challenge.json(t)["publicKey"])

	text := challenge.json(t)["challenge"].(string)
	require.True(t, strings.HasPrefix(text, "otc-desk:"))

	forged := api.do(http.MethodPost, "/api/v1/auth/login",
		`{"challenge":"`+text+`","signature":"`+b64(bytes.Repeat([]byte{1}, 64))+`","credentialId":"cred-e2e"}`, nil)
	require.Equal(t, http.StatusUnauthorized, forged.status, string(forged.body))
	require.Empty(t, forged.header.Get("Set-Cookie"))

	login := api.do(http.MethodPost, "/api/v1/auth/login",
		`{"challenge":"`+text+`","signature":"`+b64(ed25519.Sign(privateKey, []byte(text)))+`","credentialId":"cred-e2e"}`, nil)
	require.Equal(t, http.StatusUnauthorized, login.status, "a failed attempt used the challenge up")

	challenge = api.do(http.MethodPost, "/api/v1/auth/challenge", `{"credentialId":"cred-e2e"}`, nil)
	text = challenge.json(t)["challenge"].(string)

	login = api.do(http.MethodPost, "/api/v1/auth/login",
		`{"challenge":"`+text+`","signature":"`+b64(ed25519.Sign(privateKey, []byte(text)))+`","credentialId":"cred-e2e"}`, nil)
	require.Equal(t, http.StatusOK, login.status, string(login.body))
	require.Equal(t, true, login.json(t)["makerMode"])

	again := api.do(http.MethodPost, "/api/v1/auth/login",
		`{"challenge":"`+text+`","signature":"`+b64(ed25519.Sign(privateKey, []byte(text)))+`","credentialId":"cred-e2e"}`, nil)
	require.Equal(t, http.StatusUnauthorized, again.status, "a challenge works once")

	laterToken := cookieOf(t, login)
	require.NotEqual(t, token, laterToken)

	// Sign-out ends that session and only that one, cache included.
	_, err = identity.Authenticate(ctx, &identitypb.AuthenticateInput{SessionToken: laterToken})
	require.NoError(t, err)

	out := api.do(http.MethodPost, "/api/v1/auth/logout", "", map[string]string{"Cookie": "otc_session=" + laterToken})
	require.Equal(t, http.StatusNoContent, out.status)
	require.Equal(t, "otc_session=; Path=/; HttpOnly; SameSite=Lax; Max-Age=0", out.header.Get("Set-Cookie"))

	_, err = identity.Authenticate(ctx, &identitypb.AuthenticateInput{SessionToken: laterToken})
	require.Equal(t, codes.Unauthenticated, status.Code(err), "a signed-out session is dead at once, not after the cache expires")

	_, err = identity.Authenticate(ctx, &identitypb.AuthenticateInput{SessionToken: token})
	require.NoError(t, err, "the other session lives on")

	stale := api.do(http.MethodPost, "/api/v1/auth/logout", "", map[string]string{"Cookie": "otc_session=" + laterToken})
	require.Equal(t, http.StatusNoContent, stale.status, "signing out twice is fine")

	// Shutdown is graceful.
	stop()

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(15 * time.Second):
		t.Fatal("the service did not stop")
	}
}
