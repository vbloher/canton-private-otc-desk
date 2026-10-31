package metrics

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

//nolint:gochecknoglobals
var (
	httpClientRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "http_client_requests_total",
		Help: "Total number of outgoing HTTP requests",
	}, []string{labelHost, labelStatus})

	httpClientDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "http_client_request_duration_seconds",
		Help:    "Duration of outgoing HTTP requests in seconds",
		Buckets: buckets,
	}, []string{labelHost})
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func Transport(next http.RoundTripper) http.RoundTripper {
	return roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		start := time.Now()

		resp, err := next.RoundTrip(r)

		status := Error.String()
		if err == nil {
			status = strconv.Itoa(resp.StatusCode)
		}

		httpClientDuration.WithLabelValues(r.URL.Host).Observe(time.Since(start).Seconds())
		httpClientRequests.WithLabelValues(r.URL.Host, status).Inc()

		return resp, err //nolint:wrapcheck
	})
}
