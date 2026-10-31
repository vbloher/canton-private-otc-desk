package events

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/pkg/redis"
)

//nolint:gochecknoglobals
var published = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "events_published_total",
	Help: "Total number of published domain events",
}, []string{"type"})

type Publisher struct {
	redis *redis.Client
}

func NewPublisher(client *redis.Client) *Publisher {
	return &Publisher{redis: client}
}

func (p *Publisher) Publish(ctx context.Context, ev Event, partyIDs ...string) error {
	ctx, span := tracer.Start(ctx, "events Publish "+ev.Type)
	defer span.End()

	payload, err := json.Marshal(ev)
	if err != nil {
		return fmt.Errorf("json.Marshal: %w", err)
	}

	published.WithLabelValues(ev.Type).Inc()

	seen := make(map[string]struct{}, len(partyIDs))

	for _, partyID := range partyIDs {
		if _, ok := seen[partyID]; ok || partyID == "" {
			continue
		}

		seen[partyID] = struct{}{}

		err = p.redis.Publish(ctx, Channel(partyID), payload).Err()
		if err != nil {
			return fmt.Errorf("redis.Publish: %w", err)
		}
	}

	return nil
}
