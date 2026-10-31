package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

//nolint:gochecknoglobals
var (
	workerTicks = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "worker_ticks_total",
		Help: "Total number of background worker ticks",
	}, []string{labelWorker, labelStatus})

	workerTickDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "worker_tick_duration_seconds",
		Help:    "Duration of background worker ticks in seconds",
		Buckets: buckets,
	}, []string{labelWorker})
)

func WorkerTick(worker string, start time.Time, err error) {
	status := Ok
	if err != nil {
		status = Error
	}

	workerTickDuration.WithLabelValues(worker).Observe(time.Since(start).Seconds())
	workerTicks.WithLabelValues(worker, status.String()).Inc()
}
