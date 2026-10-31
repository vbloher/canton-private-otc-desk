package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

const labelName = "name"

type Process struct {
	total    *prometheus.CounterVec
	duration *prometheus.HistogramVec
	current  *prometheus.GaugeVec
}

func NewProcess() *Process {
	m := &Process{}

	m.total = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "process_total",
		Help: "Total number of processed something",
	}, []string{labelName, labelStatus})
	prometheus.MustRegister(m.total)

	m.duration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "process_duration_seconds",
		Help:    "Duration of processing something",
		Buckets: buckets,
	}, []string{labelName})
	prometheus.MustRegister(m.duration)

	m.current = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "process_current",
		Help: "Current number of processing something",
	}, []string{labelName})
	prometheus.MustRegister(m.current)

	return m
}

func (p *Process) Total(name string, status Status) {
	p.total.WithLabelValues(name, status.String()).Inc()
}

func (p *Process) TotalAdd(name string, status Status, counter int) {
	p.total.WithLabelValues(name, status.String()).Add(float64(counter))
}

func (p *Process) Duration(name string, startTime time.Time) {
	p.duration.WithLabelValues(name).Observe(time.Since(startTime).Seconds())
}

func (p *Process) Current(name string, value float64) {
	p.current.WithLabelValues(name).Set(value)
}
