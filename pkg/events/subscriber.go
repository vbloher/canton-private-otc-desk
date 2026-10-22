package events

import (
	"context"
	"strings"

	"github.com/rs/zerolog/log"

	"github.com/vbloher/canton-private-otc-desk/pkg/redis"
)

type Subscriber struct {
	redis *redis.Client
}

func NewSubscriber(client *redis.Client) *Subscriber {
	return &Subscriber{redis: client}
}

// Run delivers every event of every party to fn until ctx is done.
// The underlying go-redis subscription reconnects on its own.
func (s *Subscriber) Run(ctx context.Context, fn func(partyID string, payload []byte)) {
	sub := s.redis.PSubscribe(ctx, channelPrefix+"*")

	defer func() {
		err := sub.Close()
		if err != nil {
			log.Error().Err(err).Msg("events: subscriber close")
		}
	}()

	messages := sub.Channel()

	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-messages:
			if !ok {
				return
			}

			fn(strings.TrimPrefix(msg.Channel, channelPrefix), []byte(msg.Payload))
		}
	}
}
