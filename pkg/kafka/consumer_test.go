package kafka_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/pkg/kafka"
)

var errBusy = errors.New("database is busy")

func consumed(t *testing.T, topic, status string) float64 {
	t.Helper()

	families, err := prometheus.DefaultGatherer.Gather()
	require.NoError(t, err)

	for _, family := range families {
		if family.GetName() != "kafka_consumed_total" {
			continue
		}

		for _, m := range family.GetMetric() {
			labels := map[string]string{}
			for _, label := range m.GetLabel() {
				labels[label.GetName()] = label.GetValue()
			}

			if labels["topic"] == topic && labels["status"] == status {
				return m.GetCounter().GetValue()
			}
		}
	}

	return 0
}

func Test_Process_RetriesTheRecordUntilTheHandlerSucceeds(t *testing.T) {
	record := kafka.Record{Topic: "retry", Value: []byte("v")}
	calls := 0

	handled := kafka.Process(t.Context(), record, func(_ context.Context, r kafka.Record) error {
		calls++

		require.Equal(t, record, r)

		if calls < 3 {
			return errBusy
		}

		return nil
	}, time.Millisecond)

	require.True(t, handled)
	require.Equal(t, 3, calls)
	require.InDelta(t, 1, consumed(t, "retry", "ok"), 0)
	require.InDelta(t, 2, consumed(t, "retry", "retry"), 0)
}

func Test_Process_SkipsAPermanentFailureAndCountsIt(t *testing.T) {
	calls := 0

	handled := kafka.Process(t.Context(), kafka.Record{Topic: "poison"}, func(context.Context, kafka.Record) error {
		calls++

		return fmt.Errorf("decode: %w", kafka.ErrPermanent)
	}, time.Millisecond)

	require.True(t, handled)
	require.Equal(t, 1, calls)
	require.InDelta(t, 1, consumed(t, "poison", "skipped"), 0)
}

func Test_Process_StopsRetryingWhenTheContextIsCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	calls := 0

	handled := kafka.Process(ctx, kafka.Record{Topic: "stop"}, func(context.Context, kafka.Record) error {
		calls++

		cancel()

		return errBusy
	}, time.Hour)

	require.False(t, handled)
	require.Equal(t, 1, calls)
}
