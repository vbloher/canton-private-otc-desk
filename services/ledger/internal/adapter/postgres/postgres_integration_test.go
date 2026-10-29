//go:build integration

package postgres_test

import (
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
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/adapter/postgres"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/migration"
)

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}

	return fallback
}

type store struct {
	*postgres.Postgres

	tx *transaction.Manager
}

func newStore(t *testing.T) store {
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
	name := "ledger_it_" + hex.EncodeToString(suffix)

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

	tx := transaction.New(pool)

	return store{Postgres: postgres.New(tx), tx: tx}
}

func action(party, purpose, ref, counterparty string, createdAt time.Time, ttl time.Duration) domain.Action {
	return domain.NewAction(domain.Intent{
		PartyID: party, Purpose: purpose, Ref: ref, Counterparty: counterparty, Description: purpose + " " + ref,
	}, domain.Prepared{
		Transaction: "prepared-" + ref, HashingSchemeVersion: "HASHING_SCHEME_VERSION_V2",
		Hash: []byte{0x12, 0x20, 0x00, 0xff, 0x7f},
	}, createdAt.UTC().Truncate(time.Microsecond), ttl)
}

func Test_Actions_RoundTrip(t *testing.T) {
	s := newStore(t)
	now := time.Now()

	older := action("alice", domain.PurposeAllocate, "s-1", "", now.Add(-time.Minute), 10*time.Minute)
	newer := action("alice", domain.PurposeTransferOut, "bob::1220", "bob::1220", now, 10*time.Minute)
	stale := action("alice", domain.PurposeAllocate, "s-2", "", now.Add(-time.Hour), 10*time.Minute)
	others := action("carol", domain.PurposeAllocate, "s-1", "", now, 10*time.Minute)

	require.NoError(t, s.SaveActions(t.Context(), newer, older, stale, others))

	pending, err := s.ListPendingActions(t.Context(), "alice", now)
	require.NoError(t, err)
	require.Equal(t, []domain.Action{older, newer}, pending, "oldest first; expired ones are no longer pending")

	err = s.tx.Wrap(t.Context(), func(ctx context.Context) error {
		got, err := s.GetActionForUpdate(ctx, newer.ID)
		require.NoError(t, err)
		require.Equal(t, newer, got)
		require.Equal(t, "bob::1220", got.Counterparty)

		return s.UpdateAction(ctx, got.Finished(nil))
	})
	require.NoError(t, err)

	got, err := s.GetActionForUpdate(t.Context(), newer.ID)
	require.NoError(t, err)
	require.Equal(t, domain.ActionExecuted, got.Status)

	_, err = s.GetActionForUpdate(t.Context(), uuid.New())
	require.ErrorIs(t, err, domain.ErrNotFound)
}

func Test_ExpireActionsByRef_OnlyPendingOfThatRef(t *testing.T) {
	s := newStore(t)
	now := time.Now()

	alice := action("alice", domain.PurposeAllocate, "s-1", "", now, 10*time.Minute)
	carol := action("carol", domain.PurposeAllocate, "s-1", "", now, 10*time.Minute)
	signed := action("dave", domain.PurposeAllocate, "s-1", "", now, 10*time.Minute)
	elsewhere := action("alice", domain.PurposeAllocate, "s-2", "", now, 10*time.Minute)

	require.NoError(t, s.SaveActions(t.Context(), alice, carol, signed, elsewhere))
	require.NoError(t, s.UpdateAction(t.Context(), signed.Finished(nil)))

	expired, err := s.ExpireActionsByRef(t.Context(), "s-1")
	require.NoError(t, err)
	require.ElementsMatch(t, []uuid.UUID{alice.ID, carol.ID}, []uuid.UUID{expired[0].ID, expired[1].ID})
	require.Len(t, expired, 2)
	require.Equal(t, domain.ActionExpired, expired[0].Status)

	pending, err := s.ListPendingActions(t.Context(), "alice", now)
	require.NoError(t, err)
	require.Equal(t, []domain.Action{elsewhere}, pending)

	again, err := s.ExpireActionsByRef(t.Context(), "s-1")
	require.NoError(t, err)
	require.Empty(t, again)
}

func Test_ExpireActions_PastTheirTTL(t *testing.T) {
	s := newStore(t)
	now := time.Now()

	late := action("alice", domain.PurposeAllocate, "s-1", "", now.Add(-time.Hour), 10*time.Minute)
	fresh := action("alice", domain.PurposeAllocate, "s-2", "", now, 10*time.Minute)

	require.NoError(t, s.SaveActions(t.Context(), late, fresh))

	expired, err := s.ExpireActions(t.Context(), now)
	require.NoError(t, err)
	require.Len(t, expired, 1)
	require.Equal(t, late.ID, expired[0].ID)

	pending, err := s.ListPendingActions(t.Context(), "alice", now)
	require.NoError(t, err)
	require.Equal(t, []domain.Action{fresh}, pending)
}
