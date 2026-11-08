package kafka

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const (
	statusOK      = "ok"
	statusRetry   = "retry"
	statusSkipped = "skipped"
)

//nolint:gochecknoglobals
var (
	produced = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "kafka_produced_total",
		Help: "Total number of records written to Kafka",
	}, []string{"topic"})

	consumed = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "kafka_consumed_total",
		Help: "Total number of attempts to process a record read from Kafka",
	}, []string{"topic", "status"})
)
