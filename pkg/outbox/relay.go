package outbox

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog/log"

	"github.com/vbloher/canton-private-otc-desk/pkg/kafka"
	"github.com/vbloher/canton-private-otc-desk/pkg/metrics"
	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/pkg/transaction"
)

const (
	tickTimeout = 10 * time.Second
	cleanEvery  = time.Hour
)

var ErrInvalidConfig = errors.New("outbox: interval, batch size and retention must be positive")

type Producer interface {
	Produce(ctx context.Context, records ...kafka.Record) error
}

type Config struct {
	Interval  time.Duration `env:"OUTBOX_INTERVAL"   envDefault:"1s"`
	BatchSize int           `env:"OUTBOX_BATCH_SIZE" envDefault:"100"`
	Retention time.Duration `env:"OUTBOX_RETENTION"  envDefault:"168h"`
}

type Relay struct {
	config   Config
	tx       *transaction.Manager
	producer Producer
	cleaned  time.Time
	stop     context.CancelFunc
	done     chan struct{}
}

func NewRelay(c Config, tx *transaction.Manager, producer Producer) (*Relay, error) {
	if c.Interval <= 0 || c.BatchSize <= 0 || c.Retention <= 0 {
		return nil, fmt.Errorf("%w: %+v", ErrInvalidConfig, c)
	}

	ctx, stop := context.WithCancel(context.Background())

	r := &Relay{config: c, tx: tx, producer: producer, stop: stop, done: make(chan struct{})}

	go r.run(tracer.Suppress(ctx))

	return r, nil
}

func (r *Relay) Close() {
	r.stop()

	<-r.done

	log.Info().Msg("outbox: relay closed")
}

func (r *Relay) Tick(ctx context.Context, now time.Time) error {
	ctx, cancel := context.WithTimeout(ctx, tickTimeout)
	defer cancel()

	return errors.Join(r.clean(ctx, now), r.publish(ctx, now))
}

func (r *Relay) run(ctx context.Context) {
	defer close(r.done)

	ticker := time.NewTicker(r.config.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			start := time.Now()

			err := r.Tick(ctx, start)
			metrics.WorkerTick("outbox", start, err)

			if err != nil && ctx.Err() == nil {
				log.Error().Err(err).Msg("outbox: relay")
			}
		}
	}
}

func (r *Relay) clean(ctx context.Context, now time.Time) error {
	if now.Sub(r.cleaned) < cleanEvery {
		return nil
	}

	const sql = `DELETE FROM outbox WHERE published_at < $1`

	_, err := r.tx.TryExtractTX(ctx).Exec(ctx, sql, now.Add(-r.config.Retention))
	if err != nil {
		return fmt.Errorf("delete published rows: %w", err)
	}

	r.cleaned = now

	return nil
}

func (r *Relay) publish(ctx context.Context, now time.Time) error {
	err := r.tx.Wrap(ctx, func(ctx context.Context) error {
		ids, records, err := r.lock(ctx)
		if err != nil {
			return fmt.Errorf("r.lock: %w", err)
		}

		if len(ids) == 0 {
			return nil
		}

		ctx, span := tracer.StartRoot(ctx, "outbox Publish")
		defer span.End()

		err = r.producer.Produce(ctx, records...)
		if err != nil {
			return fmt.Errorf("producer.Produce: %w", err)
		}

		const sql = `UPDATE outbox SET published_at = $1 WHERE id = ANY($2)`

		_, err = r.tx.TryExtractTX(ctx).Exec(ctx, sql, now, ids)
		if err != nil {
			return fmt.Errorf("txOrPool.Exec: %w", err)
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("transaction.Wrap: %w", err)
	}

	return nil
}

func (r *Relay) lock(ctx context.Context) ([]int64, []kafka.Record, error) {
	const sql = `SELECT id, event_id, topic, key, type, payload, headers
                 FROM outbox
                 WHERE published_at IS NULL
                 ORDER BY id
                 LIMIT $1
                 FOR UPDATE SKIP LOCKED`

	rows, err := r.tx.TryExtractTX(ctx).Query(ctx, sql, r.config.BatchSize)
	if err != nil {
		return nil, nil, fmt.Errorf("txOrPool.Query: %w", err)
	}
	defer rows.Close()

	var (
		ids     []int64
		records []kafka.Record
	)

	for rows.Next() {
		id, record, err := scan(rows)
		if err != nil {
			return nil, nil, err
		}

		ids = append(ids, id)
		records = append(records, record)
	}

	err = rows.Err()
	if err != nil {
		return nil, nil, fmt.Errorf("rows.Err: %w", err)
	}

	return ids, records, nil
}

func scan(rows pgx.Rows) (int64, kafka.Record, error) {
	var (
		id        int64
		eventID   string
		eventType string
		record    kafka.Record
	)

	err := rows.Scan(&id, &eventID, &record.Topic, &record.Key, &eventType, &record.Value, &record.Headers)
	if err != nil {
		return 0, kafka.Record{}, fmt.Errorf("rows.Scan: %w", err)
	}

	record.Headers[HeaderEventID] = eventID
	record.Headers[HeaderEventType] = eventType

	return id, record, nil
}
