package usecase_test

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	dto "github.com/prometheus/client_model/go"
)

func gathered(t *testing.T, name string, labels map[string]string) []*dto.Metric {
	t.Helper()

	families, err := prometheus.DefaultGatherer.Gather()
	require.NoError(t, err)

	var out []*dto.Metric

	for _, family := range families {
		if family.GetName() != name {
			continue
		}

		for _, m := range family.GetMetric() {
			if matches(m, labels) {
				out = append(out, m)
			}
		}
	}

	return out
}

func matches(m *dto.Metric, labels map[string]string) bool {
	found := 0

	for _, label := range m.GetLabel() {
		if want, ok := labels[label.GetName()]; ok && want == label.GetValue() {
			found++
		}
	}

	return found == len(labels)
}

func counterValue(t *testing.T, name string, labels map[string]string) float64 {
	t.Helper()

	var sum float64

	for _, m := range gathered(t, name, labels) {
		sum += m.GetCounter().GetValue()
	}

	return sum
}

func histogramCount(t *testing.T, name string) uint64 {
	t.Helper()

	var sum uint64

	for _, m := range gathered(t, name, nil) {
		sum += m.GetHistogram().GetSampleCount()
	}

	return sum
}
