package redis

import (
	"github.com/vbloher/canton-private-otc-desk/pkg/redis"
)

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
