package redis

import (
	"time"

	"github.com/vbloher/canton-private-otc-desk/pkg/redis"
)

const (
	deskKey = "trading:desk"
	deskTTL = time.Minute
)

type Redis struct {
	redis *redis.Client
}

func New(client *redis.Client) *Redis {
	return &Redis{
		redis: client,
	}
}

// desk is the cached form of domain.Desk.
type desk struct {
	Operator    string   `json:"operator"`
	Instruments []string `json:"instruments"`
}
