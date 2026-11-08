//go:build integration

package outbox_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/vbloher/canton-private-otc-desk/pkg/kafka"
	"github.com/vbloher/canton-private-otc-desk/pkg/migrate"
	"github.com/vbloher/canton-private-otc-desk/pkg/outbox"
	pgpool "github.com/vbloher/canton-private-otc-desk/pkg/postgres"
	"github.com/vbloher/canton-private-otc-desk/pkg/transaction"
	"github.com/vbloher/canton-private-otc-desk/services/trading/migration"
)

const never = time.Hour

var errBroker = errors.New("the broker is down")

type store struct {
	pool *pgpool.Pool
	tx   *transaction.Manager
}

func newStore(t *testing.T) store {
	t.Helper()

	admin := pgpool.Config{User: "otc", Password: "otc", Host: "localhost", Port: "5432", DBName: "postgres"}

	adminPool, err := pgpool.New(t.Context(), admin)
	require.NoError(t, err)
	t.Cleanup(adminPool.Close)

	c := admin
	c.DBName = "outbox_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")[:12]

	_, err = adminPool.Exec(t.Context(), "CREATE DATABASE "+c.DBName)
	require.NoError(t, err)
	require.NoError(t, migrate.Up(c.URL(), migration.FS))

	pool, err := pgpool.New(t.Context(), c)
	require.NoError(t, err)

	t.Cleanup(func() {
		pool.Close()

		_, err := adminPool.Exec(context.WithoutCancel(t.Context()), "DROP DATABASE "+c.DBName+" WITH (FORCE)")
		require.NoError(t, err)
	})

	return store{pool: pool, tx: transaction.New(pool)}
}

func (s store) add(t *testing.T, keys ...string) []uuid.UUID {
	t.Helper()

	ids := make([]uuid.UUID, 0, len(keys))
	writer := outbox.NewWriter(s.tx)

	require.NoError(t, s.tx.Wrap(t.Context(), func(ctx context.Context) error {
		for _, key := range keys {
			m, err := outbox.NewMessage("topic", key, "thing.happened", map[string]string{"key": key})
			require.NoError(t, err)

			ids = append(ids, m.ID)

			err = writer.Add(ctx, m)
			if err != nil {
				return err
			}
		}

		return nil
	}))

	return ids
}

func (s store) count(t *testing.T, where string) int {
	t.Helper()

	var n int

	require.NoError(t, s.pool.QueryRow(t.Context(), "SELECT count(*) FROM outbox WHERE "+where).Scan(&n))

	return n
}

type producer struct {
	mu      sync.Mutex
	records []kafka.Record
	err     error
	entered func()
}

func (p *producer) Produce(_ context.Context, records ...kafka.Record) error {
	if p.entered != nil {
		p.entered()
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.err != nil {
		return p.err
	}

	p.records = append(p.records, records...)

	return nil
}

func (p *producer) keys() []string {
	p.mu.Lock()
	defer p.mu.Unlock()

	keys := make([]string, 0, len(p.records))
	for _, r := range p.records {
		keys = append(keys, r.Key)
	}

	return keys
}

func relay(t *testing.T, s store, p outbox.Producer, batch int) *outbox.Relay {
	t.Helper()

	r, err := outbox.NewRelay(outbox.Config{Interval: never, BatchSize: batch, Retention: 24 * time.Hour}, s.tx, p)
	require.NoError(t, err)
	t.Cleanup(r.Close)

	return r
}

func Test_Writer_RefusesToWorkOutsideATransaction(t *testing.T) {
	s := newStore(t)

	m, err := outbox.NewMessage("topic", "key", "thing.happened", struct{}{})
	require.NoError(t, err)

	require.ErrorIs(t, outbox.NewWriter(s.tx).Add(t.Context(), m), outbox.ErrNoTransaction)
	require.Zero(t, s.count(t, "true"))
}

func Test_Writer_LeavesNothingWhenTheTransactionRollsBack(t *testing.T) {
	s := newStore(t)

	m, err := outbox.NewMessage("topic", "key", "thing.happened", struct{}{})
	require.NoError(t, err)

	err = s.tx.Wrap(t.Context(), func(ctx context.Context) error {
		require.NoError(t, outbox.NewWriter(s.tx).Add(ctx, m))

		return errBroker
	})

	require.ErrorIs(t, err, errBroker)
	require.Zero(t, s.count(t, "true"))
}

func Test_Relay_PublishesInOrderAndMarksTheRows(t *testing.T) {
	s := newStore(t)
	ids := s.add(t, "first", "second")

	var p producer

	require.NoError(t, relay(t, s, &p, 10).Tick(t.Context(), time.Now()))

	require.Equal(t, []string{"first", "second"}, p.keys())
	require.Equal(t, "topic", p.records[0].Topic)
	require.JSONEq(t, `{"key":"first"}`, string(p.records[0].Value))
	require.Equal(t, ids[0].String(), p.records[0].Headers[outbox.HeaderEventID])
	require.Equal(t, "thing.happened", p.records[0].Headers[outbox.HeaderEventType])
	require.Zero(t, s.count(t, "published_at IS NULL"))
}

func Test_Relay_DoesNotPublishARowTwice(t *testing.T) {
	s := newStore(t)
	s.add(t, "first")

	var p producer

	r := relay(t, s, &p, 10)
	require.NoError(t, r.Tick(t.Context(), time.Now()))
	require.NoError(t, r.Tick(t.Context(), time.Now()))

	require.Equal(t, []string{"first"}, p.keys())
}

func Test_Relay_KeepsTheRowsWhenTheBrokerIsDownAndPublishesThemLater(t *testing.T) {
	s := newStore(t)
	s.add(t, "first")

	p := producer{err: errBroker}
	r := relay(t, s, &p, 10)

	require.ErrorIs(t, r.Tick(t.Context(), time.Now()), errBroker)
	require.Equal(t, 1, s.count(t, "published_at IS NULL"))

	p.mu.Lock()
	p.err = nil
	p.mu.Unlock()

	require.NoError(t, r.Tick(t.Context(), time.Now()))
	require.Equal(t, []string{"first"}, p.keys())
	require.Zero(t, s.count(t, "published_at IS NULL"))
}

func Test_Relay_TwoRelaysTakeDifferentRows(t *testing.T) {
	s := newStore(t)
	s.add(t, "a", "b", "c", "d")

	var arrived sync.WaitGroup

	arrived.Add(2)

	p := producer{entered: func() {
		arrived.Done()
		arrived.Wait()
	}}

	first := relay(t, s, &p, 2)
	second := relay(t, s, &p, 2)

	var ticks sync.WaitGroup

	for _, r := range []*outbox.Relay{first, second} {
		ticks.Go(func() {
			require.NoError(t, r.Tick(t.Context(), time.Now()))
		})
	}

	ticks.Wait()

	require.ElementsMatch(t, []string{"a", "b", "c", "d"}, p.keys())
}

func Test_Relay_DeletesOnlyOldPublishedRows(t *testing.T) {
	s := newStore(t)
	s.add(t, "old")

	start := time.Now()

	var p producer

	r := relay(t, s, &p, 10)
	require.NoError(t, r.Tick(t.Context(), start))

	s.add(t, "waiting")

	p.mu.Lock()
	p.err = errBroker
	p.mu.Unlock()

	require.ErrorIs(t, r.Tick(t.Context(), start.Add(48*time.Hour)), errBroker)

	require.Zero(t, s.count(t, "published_at IS NOT NULL"))
	require.Equal(t, 1, s.count(t, "published_at IS NULL"))
}

func Test_Relay_CarriesTheTraceOfTheOriginalRequest(t *testing.T) {
	otel.SetTextMapPropagator(propagation.TraceContext{})

	s := newStore(t)
	provider := sdktrace.NewTracerProvider()

	ctx, span := provider.Tracer("test").Start(t.Context(), "settlement")
	defer span.End()

	m, err := outbox.NewMessage("topic", "key", "thing.happened", struct{}{})
	require.NoError(t, err)

	require.NoError(t, s.tx.Wrap(ctx, func(ctx context.Context) error {
		return outbox.NewWriter(s.tx).Add(ctx, m)
	}))

	var p producer

	require.NoError(t, relay(t, s, &p, 10).Tick(t.Context(), time.Now()))

	require.Contains(t, p.records[0].Headers["traceparent"], span.SpanContext().TraceID().String())
}
