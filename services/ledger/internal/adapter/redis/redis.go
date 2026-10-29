package redis

import (
	"time"

	"github.com/vbloher/canton-private-otc-desk/pkg/redis"
)

const (
	synchronizerKey = "ledger:synchronizer"
	faucetKeyPrefix = "ledger:faucet:"
	ttl             = time.Hour
)

type Redis struct {
	redis *redis.Client
}

func New(client *redis.Client) *Redis {
	return &Redis{
		redis: client,
	}
}
