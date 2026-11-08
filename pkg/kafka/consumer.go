package kafka

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
)

const (
	pollSize      = 100
	firstRetry    = time.Second
	maxRetry      = 30 * time.Second
	commitTimeout = 10 * time.Second
	pollFailDelay = time.Second
)

type Consumer struct {
	client *kgo.Client
}

func NewConsumer(c Config, group string, topics ...string) (*Consumer, error) {
	client, err := kgo.NewClient(
		kgo.SeedBrokers(c.Brokers...),
		kgo.ConsumerGroup(group),
		kgo.ConsumeTopics(topics...),
		kgo.AllowAutoTopicCreation(),
		kgo.DisableAutoCommit(),
		kgo.BlockRebalanceOnPoll(),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()),
	)
	if err != nil {
		return nil, fmt.Errorf("kgo.NewClient: %w", err)
	}

	return &Consumer{client: client}, nil
}

func (c *Consumer) Run(ctx context.Context, handle Handler) {
	for ctx.Err() == nil {
		fetches := c.client.PollRecords(ctx, pollSize)
		if fetches.IsClientClosed() {
			return
		}

		c.report(ctx, fetches)
		c.commit(ctx, handled(ctx, fetches.Records(), handle))
		c.client.AllowRebalance()
	}
}

func (c *Consumer) Close() {
	c.client.Close()
}

func (c *Consumer) report(ctx context.Context, fetches kgo.Fetches) {
	failures := fetches.Errors()
	if len(failures) == 0 || ctx.Err() != nil {
		return
	}

	for _, failure := range failures {
		log.Error().Err(failure.Err).Str("topic", failure.Topic).Msg("kafka: poll")
	}

	if fetches.NumRecords() == 0 {
		wait(ctx, pollFailDelay)
	}
}

func (c *Consumer) commit(ctx context.Context, records []*kgo.Record) {
	if len(records) == 0 {
		return
	}

	commitCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), commitTimeout)
	defer cancel()

	err := c.client.CommitRecords(commitCtx, records...)
	if err != nil {
		log.Error().Err(err).Msg("kafka: commit")
	}
}

func handled(ctx context.Context, records []*kgo.Record, handle Handler) []*kgo.Record {
	done := make([]*kgo.Record, 0, len(records))

	for _, record := range records {
		if !process(ctx, fromKafka(record), handle, firstRetry) {
			break
		}

		done = append(done, record)
	}

	return done
}

func process(ctx context.Context, r Record, handle Handler, delay time.Duration) bool {
	for {
		err := attempt(ctx, r, handle)

		switch {
		case err == nil:
			consumed.WithLabelValues(r.Topic, statusOK).Inc()

			return true
		case errors.Is(err, ErrPermanent):
			consumed.WithLabelValues(r.Topic, statusSkipped).Inc()
			log.Error().Ctx(ctx).Err(err).Str("topic", r.Topic).Str("key", r.Key).Msg("kafka: record skipped")

			return true
		}

		consumed.WithLabelValues(r.Topic, statusRetry).Inc()
		log.Error().Ctx(ctx).Err(err).Str("topic", r.Topic).Str("key", r.Key).Msg("kafka: record will be retried")

		if !wait(ctx, delay) {
			return false
		}

		delay = min(delay*2, maxRetry)
	}
}

func attempt(ctx context.Context, r Record, handle Handler) (err error) {
	ctx, span := tracer.Start(tracer.Extract(ctx, r.Headers), "kafka Consume "+r.Topic)
	defer func() { tracer.End(span, err) }()

	return handle(ctx, r)
}

func wait(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
