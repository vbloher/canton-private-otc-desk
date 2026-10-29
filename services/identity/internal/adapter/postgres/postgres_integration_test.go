//go:build integration

package postgres_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/pkg/migrate"
	"github.com/vbloher/canton-private-otc-desk/pkg/otel"
	pgpool "github.com/vbloher/canton-private-otc-desk/pkg/postgres"
	"github.com/vbloher/canton-private-otc-desk/pkg/transaction"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/adapter/postgres"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/identity/migration"
)

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}

	return fallback
}

func newPostgres(t *testing.T) *postgres.Postgres {
	t.Helper()

	otel.SilentModeInit()

	cfg := pgpool.Config{
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
	name := "identity_it_" + hex.EncodeToString(suffix)

	_, err = admin.Exec(t.Context(), "CREATE DATABASE "+name)
	require.NoError(t, err)

	t.Cleanup(func() {
		ctx := context.Background()

		_, err := admin.Exec(ctx, "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)")
		require.NoError(t, err)
		require.NoError(t, admin.Close(ctx))
	})

	cfg.DBName = name

	require.NoError(t, migrate.Up(cfg.URL(), migration.FS))

	pool, err := pgpool.New(t.Context(), cfg)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	return postgres.New(transaction.New(pool))
}

func passkeyUser(t *testing.T, hint, credentialID string) domain.User {
	t.Helper()

	user, err := domain.NewPasskeyUser(
		hint+"::1220ab", hint, "Alice Smith", bytes.Repeat([]byte{7}, 32), credentialID, "blob-"+credentialID,
	)
	require.NoError(t, err)

	return user
}

func houseUser(t *testing.T, hint, apiKey string) domain.User {
	t.Helper()

	user, err := domain.NewHouseUser(hint+"::1220ab", hint, apiKey)
	require.NoError(t, err)

	return user
}

func requireSameUser(t *testing.T, want, got domain.User) {
	t.Helper()

	require.WithinDuration(t, want.CreatedAt, got.CreatedAt, time.Millisecond)

	want.CreatedAt, got.CreatedAt = time.Time{}, time.Time{}
	require.Equal(t, want, got)
}

func Test_PasskeyUser_RoundTrip(t *testing.T) {
	p := newPostgres(t)

	user := passkeyUser(t, "alice-3f9a1c", "cred-1")
	require.NoError(t, p.CreateUser(t.Context(), user))

	byID, err := p.GetUser(t.Context(), user.ID)
	require.NoError(t, err)
	requireSameUser(t, user, byID)

	byCredential, err := p.GetUserByCredential(t.Context(), "cred-1")
	require.NoError(t, err)
	requireSameUser(t, user, byCredential)

	_, err = p.GetUser(t.Context(), uuid.New())
	require.ErrorIs(t, err, domain.ErrNotFound)

	_, err = p.GetUserByCredential(t.Context(), "nope")
	require.ErrorIs(t, err, domain.ErrNotFound)

	_, err = p.GetUserByAPIKeyHash(t.Context(), domain.HashAPIKey("nope"))
	require.ErrorIs(t, err, domain.ErrNotFound)
}

func Test_CreateUser_Duplicates(t *testing.T) {
	p := newPostgres(t)

	require.NoError(t, p.CreateUser(t.Context(), passkeyUser(t, "alice-3f9a1c", "cred-1")))

	err := p.CreateUser(t.Context(), passkeyUser(t, "bob-aaaaaa", "cred-1"))
	require.ErrorIs(t, err, domain.ErrCredentialTaken, "the same passkey twice")

	err = p.CreateUser(t.Context(), passkeyUser(t, "alice-3f9a1c", "cred-2"))
	require.Error(t, err, "the same hint twice")
	require.NotErrorIs(t, err, domain.ErrCredentialTaken)
}

func Test_CreateUser_RejectsHalfBuiltUsers(t *testing.T) {
	p := newPostgres(t)

	err := p.CreateUser(t.Context(), domain.User{
		ID: uuid.New(), PartyID: "p::1", Hint: "p", DisplayName: "P", Kind: domain.KindPasskey, CreatedAt: time.Now(),
	})
	require.Error(t, err)

	err = p.CreateUser(t.Context(), domain.User{
		ID: uuid.New(), PartyID: "q::1", Hint: "q", DisplayName: "Q", Kind: domain.KindHouse, CreatedAt: time.Now(),
	})
	require.Error(t, err)
}

func Test_UpsertHouseUser(t *testing.T) {
	p := newPostgres(t)

	maker := houseUser(t, "makerA", "demo-key-a")
	require.NoError(t, p.UpsertHouseUser(t.Context(), maker))

	byKey, err := p.GetUserByAPIKeyHash(t.Context(), domain.HashAPIKey("demo-key-a"))
	require.NoError(t, err)
	requireSameUser(t, maker, byKey)
	require.Equal(t, domain.KindHouse, byKey.Kind)
	require.True(t, byKey.MakerMode)
	require.Nil(t, byKey.PublicKey)
	require.Empty(t, byKey.CredentialID)

	_, err = p.SetMakerMode(t.Context(), maker.ID, false)
	require.NoError(t, err)

	again := houseUser(t, "makerA", "rotated-key")
	again.PartyID = "makerA::9999"
	require.NoError(t, p.UpsertHouseUser(t.Context(), again))

	updated, err := p.GetUserByAPIKeyHash(t.Context(), domain.HashAPIKey("rotated-key"))
	require.NoError(t, err)
	require.Equal(t, maker.ID, updated.ID, "the id survives a re-seed")
	require.Equal(t, "makerA::9999", updated.PartyID)
	require.True(t, updated.MakerMode)

	_, err = p.GetUserByAPIKeyHash(t.Context(), domain.HashAPIKey("demo-key-a"))
	require.ErrorIs(t, err, domain.ErrNotFound, "the old key stops working")
}

func Test_UpsertHouseUser_NeverOverwritesAWallet(t *testing.T) {
	p := newPostgres(t)

	wallet := passkeyUser(t, "alice-3f9a1c", "cred-1")
	require.NoError(t, p.CreateUser(t.Context(), wallet))

	err := p.UpsertHouseUser(t.Context(), houseUser(t, "alice-3f9a1c", "some-key"))
	require.Error(t, err)
	require.ErrorContains(t, err, "alice-3f9a1c")

	got, err := p.GetUser(t.Context(), wallet.ID)
	require.NoError(t, err)
	requireSameUser(t, wallet, got)

	_, err = p.GetUserByAPIKeyHash(t.Context(), domain.HashAPIKey("some-key"))
	require.ErrorIs(t, err, domain.ErrNotFound, "the clashing key was not stored either")
}

func Test_ListMakers_And_SetMakerMode(t *testing.T) {
	p := newPostgres(t)

	alice := passkeyUser(t, "alice-3f9a1c", "cred-1")
	require.NoError(t, p.CreateUser(t.Context(), alice))
	require.NoError(t, p.UpsertHouseUser(t.Context(), houseUser(t, "makerB", "key-b")))
	require.NoError(t, p.UpsertHouseUser(t.Context(), houseUser(t, "makerA", "key-a")))

	makers, err := p.ListMakers(t.Context())
	require.NoError(t, err)
	require.Len(t, makers, 2, "a wallet is not a maker until it opts in")
	require.Equal(t, "makerA", makers[0].Hint, "sorted by name")
	require.Equal(t, "makerB", makers[1].Hint)

	updated, err := p.SetMakerMode(t.Context(), alice.ID, true)
	require.NoError(t, err)
	require.True(t, updated.MakerMode)
	require.Equal(t, alice.Hint, updated.Hint)

	makers, err = p.ListMakers(t.Context())
	require.NoError(t, err)
	require.Len(t, makers, 3)

	_, err = p.SetMakerMode(t.Context(), alice.ID, false)
	require.NoError(t, err)

	makers, err = p.ListMakers(t.Context())
	require.NoError(t, err)
	require.Len(t, makers, 2)

	_, err = p.SetMakerMode(t.Context(), uuid.New(), true)
	require.ErrorIs(t, err, domain.ErrNotFound)
}

func Test_ListMakers_Empty(t *testing.T) {
	p := newPostgres(t)

	makers, err := p.ListMakers(t.Context())
	require.NoError(t, err)
	require.NotNil(t, makers)
	require.Empty(t, makers)
}

func Test_Sessions(t *testing.T) {
	p := newPostgres(t)

	alice := passkeyUser(t, "alice-3f9a1c", "cred-1")
	require.NoError(t, p.CreateUser(t.Context(), alice))

	now := time.Now()
	hash := domain.HashSessionToken("token")

	require.NoError(t, p.CreateSession(t.Context(), hash, alice.ID, now.Add(time.Hour)))

	got, err := p.GetUserBySession(t.Context(), hash, now)
	require.NoError(t, err)
	requireSameUser(t, alice, got)

	_, err = p.GetUserBySession(t.Context(), hash, now.Add(2*time.Hour))
	require.ErrorIs(t, err, domain.ErrNotFound, "an expired session finds nobody")

	_, err = p.GetUserBySession(t.Context(), domain.HashSessionToken("other"), now)
	require.ErrorIs(t, err, domain.ErrNotFound)

	require.NoError(t, p.DeleteSession(t.Context(), hash))
	require.NoError(t, p.DeleteSession(t.Context(), hash), "ending a session twice is fine")

	_, err = p.GetUserBySession(t.Context(), hash, now)
	require.ErrorIs(t, err, domain.ErrNotFound)
}

func Test_CreateSession_NeedsAUser(t *testing.T) {
	p := newPostgres(t)

	err := p.CreateSession(t.Context(), domain.HashSessionToken("token"), uuid.New(), time.Now().Add(time.Hour))
	require.Error(t, err)
}

func Test_DeleteSession_LeavesOtherSessionsAlone(t *testing.T) {
	p := newPostgres(t)

	alice := passkeyUser(t, "alice-3f9a1c", "cred-1")
	require.NoError(t, p.CreateUser(t.Context(), alice))

	expires := time.Now().Add(time.Hour)
	require.NoError(t, p.CreateSession(t.Context(), domain.HashSessionToken("laptop"), alice.ID, expires))
	require.NoError(t, p.CreateSession(t.Context(), domain.HashSessionToken("phone"), alice.ID, expires))

	require.NoError(t, p.DeleteSession(t.Context(), domain.HashSessionToken("laptop")))

	_, err := p.GetUserBySession(t.Context(), domain.HashSessionToken("phone"), time.Now())
	require.NoError(t, err)
}
