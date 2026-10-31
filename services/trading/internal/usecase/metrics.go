package usecase

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

//nolint:gochecknoglobals
var (
	tradesTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "trading_trades_total",
		Help: "Total number of trades that settled or failed",
	}, []string{"status"})

	settlementDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "trading_settlement_duration_seconds",
		Help:    "Time from trade creation to settlement in seconds",
		Buckets: []float64{1, 2, 5, 10, 20, 30, 60, 120, 300, 600},
	})
)
