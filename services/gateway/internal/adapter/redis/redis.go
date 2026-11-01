package redis

import (
	"github.com/vbloher/canton-private-otc-desk/pkg/redis"
)

type Redis struct {
	redis *redis.Client
}

func New(client *redis.Client) *Redis {
	return &Redis{
		redis: client,
	}
}
