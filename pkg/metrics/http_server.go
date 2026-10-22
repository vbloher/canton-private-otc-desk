package metrics

import (
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

type HTTPServer struct {
	total    *prometheus.CounterVec
	duration *prometheus.HistogramVec
}

func NewHTTPServer() *HTTPServer {
	m := &HTTPServer{}

	m.total = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "http_server_requests_total",
		Help: "Total number of HTTP requests",
	}, []string{"method", "status"})
	prometheus.MustRegister(m.total)

	m.duration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "http_server_request_duration_seconds",
		Help:    "Duration of HTTP requests in seconds",
		Buckets: buckets,
	}, []string{"method"})
	prometheus.MustRegister(m.duration)

	return m
}

func (m *HTTPServer) TotalInc(method string, code int) {
	m.total.WithLabelValues(method, strconv.Itoa(code)).Inc()
}

func (m *HTTPServer) Duration(method string, startTime time.Time) {
	m.duration.WithLabelValues(method).Observe(time.Since(startTime).Seconds())
}
