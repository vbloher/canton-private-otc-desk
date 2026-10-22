package redis

import (
	"github.com/vbloher/canton-private-otc-desk/pkg/redis"
)

// The Redis server is shared with the other services, so every key carries the service name.
const (
	registrationPrefix = "identity:registration:"
	challengePrefix    = "identity:challenge:"
	sessionPrefix      = "identity:session:"
)

type Redis struct {
	redis *redis.Client
}

func New(client *redis.Client) *Redis {
	return &Redis{
		redis: client,
	}
}
