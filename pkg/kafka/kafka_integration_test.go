//go:build integration

package kafka_test

import (
	"context"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/pkg/kafka"
)

func config() kafka.Config {
	brokers := os.Getenv("KAFKA_BROKERS")
	if brokers == "" {
		brokers = "localhost:9094"
	}

	return kafka.Config{Brokers: strings.Split(brokers, ",")}
}

type collector struct {
	mu      sync.Mutex
	records []kafka.Record
}

func (c *collector) handle(_ context.Context, r kafka.Record) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.records = append(c.records, r)

	return nil
}

func (c *collector) seen() []kafka.Record {
	c.mu.Lock()
	defer c.mu.Unlock()

	return append([]kafka.Record(nil), c.records...)
}

func run(t *testing.T, group, topic string, c *collector) (stop func()) {
	t.Helper()

	consumer, err := kafka.NewConsumer(config(), group, topic)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	go func() {
		defer close(done)

		consumer.Run(ctx, c.handle)
	}()

	var once sync.Once

	stop = func() {
		once.Do(func() {
			cancel()
			<-done
			consumer.Close()
		})
	}
	t.Cleanup(stop)

	return stop
}

func Test_Kafka_DeliversRecordsWithKeyAndHeaders(t *testing.T) {
	topic := "it-" + uuid.NewString()

	producer, err := kafka.NewProducer(config())
	require.NoError(t, err)
	t.Cleanup(producer.Close)

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	want := kafka.Record{Topic: topic, Key: "k-1", Value: []byte(`{"a":1}`), Headers: map[string]string{"event-type": "t"}}
	require.NoError(t, producer.Produce(ctx, want))

	var c collector

	run(t, "group-"+topic, topic, &c)

	require.Eventually(t, func() bool { return len(c.seen()) == 1 }, 30*time.Second, 50*time.Millisecond)
	require.Equal(t, []kafka.Record{want}, c.seen())
}

func Test_Kafka_CommitsSoTheGroupDoesNotReceiveARecordAgain(t *testing.T) {
	topic := "it-" + uuid.NewString()
	group := "group-" + topic

	producer, err := kafka.NewProducer(config())
	require.NoError(t, err)
	t.Cleanup(producer.Close)

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	require.NoError(t, producer.Produce(ctx, kafka.Record{Topic: topic, Key: "1", Value: []byte("one")}))

	var first collector

	stop := run(t, group, topic, &first)

	require.Eventually(t, func() bool { return len(first.seen()) == 1 }, 30*time.Second, 50*time.Millisecond)
	stop()

	require.NoError(t, producer.Produce(ctx, kafka.Record{Topic: topic, Key: "2", Value: []byte("two")}))

	var second collector

	run(t, group, topic, &second)

	require.Eventually(t, func() bool { return len(second.seen()) == 1 }, 30*time.Second, 50*time.Millisecond)
	require.Equal(t, "2", second.seen()[0].Key)
}
