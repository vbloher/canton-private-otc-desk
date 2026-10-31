package metrics

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"

	dto "github.com/prometheus/client_model/go"
)

func get(t *testing.T, rawURL string) {
	t.Helper()

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, rawURL, nil)
	require.NoError(t, err)

	resp, err := (&http.Client{Transport: Transport(http.DefaultTransport)}).Do(req)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
}

func serve(t *testing.T, code int) string {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(code) }))
	t.Cleanup(srv.Close)

	u, err := url.Parse(srv.URL)
	require.NoError(t, err)

	return u.Host
}

func TestTransport_CountsStatus(t *testing.T) {
	host := serve(t, http.StatusServiceUnavailable)

	get(t, "http://"+host+"/readyz")

	require.InDelta(t, 1, testutil.ToFloat64(httpClientRequests.WithLabelValues(host, "503")), 0)
	require.Equal(t, uint64(1), sampleCount(t, httpClientDuration.WithLabelValues(host)))
}

func TestTransport_CountsTransportError(t *testing.T) {
	want := errors.New("dial failed")
	next := roundTripperFunc(func(*http.Request) (*http.Response, error) { return nil, want })

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://canton.test/v2/state", nil)
	require.NoError(t, err)

	_, err = Transport(next).RoundTrip(req) //nolint:bodyclose
	require.ErrorIs(t, err, want)

	require.InDelta(t, 1, testutil.ToFloat64(httpClientRequests.WithLabelValues("canton.test", "error")), 0)
}

func TestTransport_LabelsHostOnly(t *testing.T) {
	host := serve(t, http.StatusOK)

	get(t, "http://"+host+"/v2/parties/alice::1220?filter=x")

	ch := make(chan prometheus.Metric, 100)
	httpClientRequests.Collect(ch)
	close(ch)

	for m := range ch {
		var out dto.Metric

		require.NoError(t, m.Write(&out))

		for _, label := range out.GetLabel() {
			require.NotContains(t, label.GetValue(), "alice")
		}
	}

	require.InDelta(t, 1, testutil.ToFloat64(httpClientRequests.WithLabelValues(host, "200")), 0)
}
