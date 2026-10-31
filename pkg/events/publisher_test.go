package events_test

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/pkg/events"
)

func published(t *testing.T, eventType string) float64 {
	t.Helper()

	families, err := prometheus.DefaultGatherer.Gather()
	require.NoError(t, err)

	for _, family := range families {
		if family.GetName() != "events_published_total" {
			continue
		}

		for _, m := range family.GetMetric() {
			for _, label := range m.GetLabel() {
				if label.GetName() == "type" && label.GetValue() == eventType {
					return m.GetCounter().GetValue()
				}
			}
		}
	}

	return 0
}

func TestPublish_CountsEvent(t *testing.T) {
	require.NoError(t, events.NewPublisher(nil).Publish(t.Context(), events.Event{Type: "test.counted"}))

	require.InDelta(t, 1, published(t, "test.counted"), 0)
}
