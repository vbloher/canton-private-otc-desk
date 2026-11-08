package kafka

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	kafkalib "github.com/vbloher/canton-private-otc-desk/pkg/kafka"
	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/pkg/outbox"
	"github.com/vbloher/canton-private-otc-desk/pkg/tradeevents"
)

type Config struct {
	Group string `env:"KAFKA_CONSUMER_GROUP" envDefault:"notification"`
}

type Notifier interface {
	NotifyTradeSettled(ctx context.Context, event tradeevents.Settled) error
}

type Consumer struct {
	consumer *kafkalib.Consumer
	stop     context.CancelFunc
	done     chan struct{}
}

func New(brokers kafkalib.Config, c Config, notifier Notifier) (*Consumer, error) {
	consumer, err := kafkalib.NewConsumer(brokers, c.Group, tradeevents.Topic)
	if err != nil {
		return nil, fmt.Errorf("kafka.NewConsumer: %w", err)
	}

	ctx, stop := context.WithCancel(context.Background())

	result := &Consumer{consumer: consumer, stop: stop, done: make(chan struct{})}

	go func() {
		defer close(result.done)

		consumer.Run(tracer.Suppress(ctx), Handler(notifier))
	}()

	return result, nil
}

func (c *Consumer) Close() {
	c.stop()

	<-c.done

	c.consumer.Close()

	log.Info().Msg("kafka: consumer closed")
}

func Handler(notifier Notifier) kafkalib.Handler {
	return func(ctx context.Context, r kafkalib.Record) error {
		if r.Headers[outbox.HeaderEventType] != tradeevents.TypeSettled {
			return nil
		}

		var event tradeevents.Settled

		err := json.Unmarshal(r.Value, &event)
		if err != nil {
			return fmt.Errorf("%w: %w", kafkalib.ErrPermanent, err)
		}

		if event.TradeID == uuid.Nil || event.Taker.PartyID == "" || event.Maker.PartyID == "" {
			return fmt.Errorf("%w: the event names no trade or party", kafkalib.ErrPermanent)
		}

		err = notifier.NotifyTradeSettled(ctx, event)
		if err != nil {
			return fmt.Errorf("notifier.NotifyTradeSettled: %w", err)
		}

		return nil
	}
}
