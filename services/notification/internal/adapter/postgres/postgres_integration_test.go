//go:build integration

package postgres_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/pkg/migrate"
	"github.com/vbloher/canton-private-otc-desk/pkg/otel"
	pgpool "github.com/vbloher/canton-private-otc-desk/pkg/postgres"
	"github.com/vbloher/canton-private-otc-desk/pkg/transaction"
	"github.com/vbloher/canton-private-otc-desk/services/notification/internal/adapter/postgres"
	"github.com/vbloher/canton-private-otc-desk/services/notification/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/notification/migration"
)

func connect(t *testing.T) (*postgres.Postgres, *transaction.Manager) {
	t.Helper()

	otel.SilentModeInit()

	admin := pgpool.Config{User: "otc", Password: "otc", Host: "localhost", Port: "5432", DBName: "postgres"}

	adminPool, err := pgpool.New(t.Context(), admin)
	require.NoError(t, err)
	t.Cleanup(adminPool.Close)

	c := admin
	c.DBName = "notification_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")[:12]

	require.NoError(t, pgpool.EnsureDatabase(t.Context(), c))
	require.NoError(t, pgpool.EnsureDatabase(t.Context(), c), "an existing database is fine")
	require.NoError(t, migrate.Up(c.URL(), migration.FS))

	pool, err := pgpool.New(t.Context(), c)
	require.NoError(t, err)

	t.Cleanup(func() {
		pool.Close()

		_, err := adminPool.Exec(context.WithoutCancel(t.Context()), "DROP DATABASE "+c.DBName+" WITH (FORCE)")
		require.NoError(t, err)
	})

	tx := transaction.New(pool)

	return postgres.New(tx), tx
}

func confirmed(party, email string, news bool) domain.Profile {
	at := time.Now().UTC().Truncate(time.Microsecond)

	return domain.Profile{PartyID: party, Email: email, ConfirmedAt: &at, News: news}
}

func letter(key string, now time.Time) domain.Email {
	return domain.NewEmail(key, "a@b.co", domain.KindConfirmation, domain.Letter{Subject: "s", HTML: "h", Text: "t"}, now)
}

func Test_Profile_RoundTrip(t *testing.T) {
	pg, _ := connect(t)
	ctx := t.Context()

	_, err := pg.GetProfile(ctx, "alice")
	require.ErrorIs(t, err, domain.ErrNotFound)

	want := confirmed("alice", "a@b.co", true)
	want.Trades = true
	require.NoError(t, pg.SaveProfile(ctx, want))

	got, err := pg.GetProfile(ctx, "alice")
	require.NoError(t, err)
	require.Equal(t, want.Email, got.Email)
	require.True(t, want.ConfirmedAt.Equal(*got.ConfirmedAt))
	require.True(t, got.News)
	require.True(t, got.Trades)

	require.NoError(t, pg.SaveProfile(ctx, domain.Profile{PartyID: "alice", Email: "new@b.co"}))

	got, err = pg.GetProfile(ctx, "alice")
	require.NoError(t, err)
	require.Equal(t, domain.Profile{PartyID: "alice", Email: "new@b.co"}, got)
}

func Test_ListProfiles_ReturnsThoseThatExist(t *testing.T) {
	pg, _ := connect(t)
	ctx := t.Context()

	require.NoError(t, pg.SaveProfile(ctx, confirmed("alice", "a@b.co", false)))

	got, err := pg.ListProfiles(ctx, []string{"alice", "bob"})
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, "alice", got[0].PartyID)
}

func Test_Confirmation_ANewOneReplacesTheOldOnes(t *testing.T) {
	pg, tx := connect(t)
	ctx := t.Context()
	now := time.Now()

	require.NoError(t, pg.SaveProfile(ctx, domain.Profile{PartyID: "alice", Email: "a@b.co"}))

	first, _ := domain.NewConfirmation("alice", "a@b.co", now, time.Hour)
	second, _ := domain.NewConfirmation("alice", "a@b.co", now, time.Hour)

	require.NoError(t, pg.ReplaceConfirmation(ctx, first))
	require.NoError(t, pg.ReplaceConfirmation(ctx, second))

	require.NoError(t, tx.Wrap(ctx, func(ctx context.Context) error {
		_, err := pg.GetConfirmation(ctx, first.TokenHash)
		require.ErrorIs(t, err, domain.ErrNotFound)

		got, err := pg.GetConfirmation(ctx, second.TokenHash)
		require.NoError(t, err)
		require.Equal(t, "alice", got.PartyID)
		require.Equal(t, "a@b.co", got.Email)
		require.Nil(t, got.ConfirmedAt)

		return pg.MarkConfirmed(ctx, second.TokenHash, now)
	}))

	require.NoError(t, tx.Wrap(ctx, func(ctx context.Context) error {
		got, err := pg.GetConfirmation(ctx, second.TokenHash)
		require.NoError(t, err)
		require.NotNil(t, got.ConfirmedAt)

		return nil
	}))
}

func Test_CreateEmail_ASecondLetterWithTheSameKeyIsDropped(t *testing.T) {
	pg, tx := connect(t)
	ctx := t.Context()
	now := time.Now()

	require.NoError(t, pg.CreateEmail(ctx, letter("trade-1", now)))
	require.NoError(t, pg.CreateEmail(ctx, letter("trade-1", now)))

	seen := 0

	for range 2 {
		require.NoError(t, tx.Wrap(ctx, func(ctx context.Context) error {
			e, err := pg.NextDueEmail(ctx, now)
			if err != nil {
				require.ErrorIs(t, err, domain.ErrNotFound)

				return nil
			}

			seen++

			return pg.UpdateEmail(ctx, e.Delivered("id", now))
		}))
	}

	require.Equal(t, 1, seen)
}

func Test_NextDueEmail_TakesOnlyWhatIsDue(t *testing.T) {
	pg, tx := connect(t)
	ctx := t.Context()
	now := time.Now()

	require.NoError(t, pg.CreateEmail(ctx, letter("later", now.Add(time.Hour))))

	require.NoError(t, tx.Wrap(ctx, func(ctx context.Context) error {
		_, err := pg.NextDueEmail(ctx, now)
		require.ErrorIs(t, err, domain.ErrNotFound)

		return nil
	}))
}

func Test_NextDueEmail_TwoWorkersTakeDifferentLetters(t *testing.T) {
	pg, tx := connect(t)
	ctx := t.Context()
	now := time.Now()

	require.NoError(t, pg.CreateEmail(ctx, letter("one", now.Add(-time.Minute))))
	require.NoError(t, pg.CreateEmail(ctx, letter("two", now)))

	require.NoError(t, tx.Wrap(ctx, func(ctx context.Context) error {
		first, err := pg.NextDueEmail(ctx, now)
		require.NoError(t, err)
		require.Equal(t, "one", first.DedupeKey)

		return tx2(t, tx, pg, now, "two")
	}))
}

func tx2(t *testing.T, tx *transaction.Manager, pg *postgres.Postgres, now time.Time, want string) error {
	t.Helper()

	return tx.Wrap(context.Background(), func(ctx context.Context) error {
		second, err := pg.NextDueEmail(ctx, now)
		require.NoError(t, err)
		require.Equal(t, want, second.DedupeKey)

		return nil
	})
}

func Test_UpdateEmail_StoresARetry(t *testing.T) {
	pg, tx := connect(t)
	ctx := t.Context()
	now := time.Now().UTC().Truncate(time.Microsecond)

	require.NoError(t, pg.CreateEmail(ctx, letter("k", now)))

	require.NoError(t, tx.Wrap(ctx, func(ctx context.Context) error {
		e, err := pg.NextDueEmail(ctx, now)
		require.NoError(t, err)

		return pg.UpdateEmail(ctx, e.Retry("busy", now))
	}))

	require.NoError(t, tx.Wrap(ctx, func(ctx context.Context) error {
		e, err := pg.NextDueEmail(ctx, now.Add(31*time.Second))
		require.NoError(t, err)
		require.Equal(t, 1, e.Attempts)
		require.Equal(t, "busy", e.LastError)
		require.Equal(t, "a@b.co", e.Recipient)
		require.Equal(t, "h", e.HTML)

		return nil
	}))
}

func Test_ContactSync_QueuesOncePerAddressAndCanBeRescheduled(t *testing.T) {
	pg, tx := connect(t)
	ctx := t.Context()
	now := time.Now().UTC().Truncate(time.Microsecond)

	require.NoError(t, pg.QueueContactSync(ctx, "a@b.co", now))
	require.NoError(t, pg.QueueContactSync(ctx, "a@b.co", now))

	require.NoError(t, tx.Wrap(ctx, func(ctx context.Context) error {
		s, err := pg.NextDueContactSync(ctx, now)
		require.NoError(t, err)
		require.Equal(t, "a@b.co", s.Email)

		return pg.RescheduleContactSync(ctx, s.Retry(now))
	}))

	require.NoError(t, tx.Wrap(ctx, func(ctx context.Context) error {
		_, err := pg.NextDueContactSync(ctx, now)
		require.ErrorIs(t, err, domain.ErrNotFound)

		s, err := pg.NextDueContactSync(ctx, now.Add(time.Minute))
		require.NoError(t, err)
		require.Equal(t, 1, s.Attempts)

		return pg.DeleteContactSync(ctx, s.Email)
	}))

	require.NoError(t, tx.Wrap(ctx, func(ctx context.Context) error {
		_, err := pg.NextDueContactSync(ctx, now.Add(time.Hour))
		require.ErrorIs(t, err, domain.ErrNotFound)

		return nil
	}))
}

func Test_QueueContactSync_ANewChangeResetsARetryingSync(t *testing.T) {
	pg, tx := connect(t)
	ctx := t.Context()
	now := time.Now().UTC().Truncate(time.Microsecond)

	require.NoError(t, pg.QueueContactSync(ctx, "a@b.co", now))
	require.NoError(t, tx.Wrap(ctx, func(ctx context.Context) error {
		s, err := pg.NextDueContactSync(ctx, now)
		require.NoError(t, err)

		return pg.RescheduleContactSync(ctx, s.Retry(now))
	}))

	require.NoError(t, pg.QueueContactSync(ctx, "a@b.co", now))

	require.NoError(t, tx.Wrap(ctx, func(ctx context.Context) error {
		s, err := pg.NextDueContactSync(ctx, now)
		require.NoError(t, err)
		require.Zero(t, s.Attempts)

		return nil
	}))
}

func Test_NewsWanted_NeedsAConfirmedProfileThatAsksForNews(t *testing.T) {
	pg, _ := connect(t)
	ctx := t.Context()

	require.NoError(t, pg.SaveProfile(ctx, domain.Profile{PartyID: "unconfirmed", Email: "a@b.co", News: true}))
	require.NoError(t, pg.SaveProfile(ctx, confirmed("no-news", "a@b.co", false)))

	wanted, err := pg.NewsWanted(ctx, "a@b.co")
	require.NoError(t, err)
	require.False(t, wanted)

	require.NoError(t, pg.SaveProfile(ctx, confirmed("reader", "a@b.co", true)))

	wanted, err = pg.NewsWanted(ctx, "a@b.co")
	require.NoError(t, err)
	require.True(t, wanted)
}

func Test_ClearNews_ClearsEveryProfileWithTheAddressAndNamesThem(t *testing.T) {
	pg, _ := connect(t)
	ctx := t.Context()

	require.NoError(t, pg.SaveProfile(ctx, confirmed("alice", "a@b.co", true)))
	require.NoError(t, pg.SaveProfile(ctx, confirmed("alice-2", "a@b.co", true)))
	require.NoError(t, pg.SaveProfile(ctx, confirmed("quiet", "a@b.co", false)))
	require.NoError(t, pg.SaveProfile(ctx, confirmed("other", "other@b.co", true)))

	parties, err := pg.ClearNews(ctx, "a@b.co")
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"alice", "alice-2"}, parties)

	other, err := pg.GetProfile(ctx, "other")
	require.NoError(t, err)
	require.True(t, other.News)
}

func Test_Cleanup_KeepsWhatIsStillUseful(t *testing.T) {
	pg, tx := connect(t)
	ctx := t.Context()
	now := time.Now().UTC().Truncate(time.Microsecond)

	require.NoError(t, pg.SaveProfile(ctx, domain.Profile{PartyID: "alice", Email: "a@b.co"}))

	expired, _ := domain.NewConfirmation("alice", "a@b.co", now.Add(-2*time.Hour), time.Hour)
	require.NoError(t, pg.ReplaceConfirmation(ctx, expired))

	require.NoError(t, pg.CreateEmail(ctx, letter("old", now)))
	require.NoError(t, pg.CreateEmail(ctx, letter("waiting", now.Add(time.Minute))))

	require.NoError(t, tx.Wrap(ctx, func(ctx context.Context) error {
		e, err := pg.NextDueEmail(ctx, now)
		require.NoError(t, err)

		return pg.UpdateEmail(ctx, e.Delivered("id", now.Add(-48*time.Hour)))
	}))

	require.NoError(t, pg.Cleanup(ctx, now, now.Add(-24*time.Hour)))

	require.NoError(t, tx.Wrap(ctx, func(ctx context.Context) error {
		_, err := pg.GetConfirmation(ctx, expired.TokenHash)
		require.ErrorIs(t, err, domain.ErrNotFound)

		pending, err := pg.NextDueEmail(ctx, now.Add(time.Hour))
		require.NoError(t, err)
		require.Equal(t, "waiting", pending.DedupeKey)

		return nil
	}))

	require.NoError(t, pg.CreateEmail(ctx, letter("old", now)))

	require.NoError(t, tx.Wrap(ctx, func(ctx context.Context) error {
		again, err := pg.NextDueEmail(ctx, now)
		require.NoError(t, err, "the finished row is gone, so its key is free again")
		require.Equal(t, "old", again.DedupeKey)

		return nil
	}))
}
