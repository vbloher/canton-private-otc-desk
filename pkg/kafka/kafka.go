package kafka

import (
	"context"
	"errors"

	"github.com/twmb/franz-go/pkg/kgo"
)

var ErrPermanent = errors.New("the record can never be processed")

type Config struct {
	Brokers []string `env:"KAFKA_BROKERS,notEmpty" envSeparator:","`
}

type Record struct {
	Topic   string
	Key     string
	Value   []byte
	Headers map[string]string
}

type Handler func(ctx context.Context, r Record) error

func toKafka(r Record) *kgo.Record {
	record := &kgo.Record{Topic: r.Topic, Key: []byte(r.Key), Value: r.Value}

	for key, value := range r.Headers {
		record.Headers = append(record.Headers, kgo.RecordHeader{Key: key, Value: []byte(value)})
	}

	return record
}

func fromKafka(r *kgo.Record) Record {
	record := Record{Topic: r.Topic, Key: string(r.Key), Value: r.Value}

	if len(r.Headers) > 0 {
		record.Headers = make(map[string]string, len(r.Headers))
	}

	for _, header := range r.Headers {
		record.Headers[header.Key] = string(header.Value)
	}

	return record
}
