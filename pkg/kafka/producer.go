package kafka

import (
	"context"
	"fmt"

	"github.com/twmb/franz-go/pkg/kgo"
)

type Producer struct {
	client *kgo.Client
}

func NewProducer(c Config) (*Producer, error) {
	client, err := kgo.NewClient(
		kgo.SeedBrokers(c.Brokers...),
		kgo.AllowAutoTopicCreation(),
		kgo.AllowIdempotentProduceCancellation(),
	)
	if err != nil {
		return nil, fmt.Errorf("kgo.NewClient: %w", err)
	}

	return &Producer{client: client}, nil
}

func (p *Producer) Produce(ctx context.Context, records ...Record) error {
	batch := make([]*kgo.Record, 0, len(records))

	for _, r := range records {
		batch = append(batch, toKafka(r))
	}

	err := p.client.ProduceSync(ctx, batch...).FirstErr()
	if err != nil {
		return fmt.Errorf("client.ProduceSync: %w", err)
	}

	for _, r := range records {
		produced.WithLabelValues(r.Topic).Inc()
	}

	return nil
}

func (p *Producer) Close() {
	p.client.Close()
}
