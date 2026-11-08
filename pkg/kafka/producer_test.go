package kafka_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/twmb/franz-go/pkg/kfake"
	"github.com/twmb/franz-go/pkg/kmsg"

	"github.com/vbloher/canton-private-otc-desk/pkg/kafka"
)

func Test_Produce_GivesUpAtTheDeadlineWhenTheBrokerStopsAnswering(t *testing.T) {
	cluster, err := kfake.NewCluster(kfake.NumBrokers(1), kfake.AllowAutoTopicCreation())
	require.NoError(t, err)

	var hung atomic.Bool

	release := make(chan struct{})

	cluster.ControlKey(int16(kmsg.Produce), func(kmsg.Request) (kmsg.Response, error, bool) {
		cluster.KeepControl()

		if hung.Load() {
			<-release
		}

		return nil, nil, false
	})

	producer, err := kafka.NewProducer(kafka.Config{Brokers: cluster.ListenAddrs()})
	require.NoError(t, err)

	t.Cleanup(func() {
		close(release)
		producer.Close()
		cluster.Close()
	})

	record := kafka.Record{Topic: "stuck", Key: "k", Value: []byte("v")}

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	require.NoError(t, producer.Produce(ctx, record))

	hung.Store(true)

	result := make(chan error, 1)

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()

		result <- producer.Produce(ctx, record)
	}()

	select {
	case err := <-result:
		require.ErrorIs(t, err, context.DeadlineExceeded)
	case <-time.After(8 * time.Second):
		require.Fail(t, "Produce kept waiting for a broker that does not answer")
	}
}
