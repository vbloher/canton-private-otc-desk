package redis

import (
	"time"

	"github.com/vbloher/canton-private-otc-desk/pkg/redis"
)

const (
	rulesKey        = "ledger:token-rules"
	synchronizerKey = "ledger:synchronizer"
	ttl             = time.Hour
)

// Redis caches what every command needs from the ledger and rarely changes. It fails open:
// an unavailable Redis only means reading the ledger again.
type Redis struct {
	redis *redis.Client
}

func New(client *redis.Client) *Redis {
	return &Redis{
		redis: client,
	}
}

// disclosed is the cached form of domain.Disclosed.
type disclosed struct {
	TemplateID       string `json:"templateId"`
	ContractID       string `json:"contractId"`
	CreatedEventBlob string `json:"createdEventBlob"`
	SynchronizerID   string `json:"synchronizerId"`
}
