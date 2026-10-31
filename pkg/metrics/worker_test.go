package metrics

import (
	"errors"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
)

func TestWorkerTick_CountsStatus(t *testing.T) {
	start := time.Now()

	WorkerTick("test_ok", start, nil)
	WorkerTick("test_err", start, errors.New("x"))

	require.InDelta(t, 1, testutil.ToFloat64(workerTicks.WithLabelValues("test_ok", "ok")), 0)
	require.InDelta(t, 1, testutil.ToFloat64(workerTicks.WithLabelValues("test_err", "error")), 0)
	require.Equal(t, uint64(1), sampleCount(t, workerTickDuration.WithLabelValues("test_ok")))
}
