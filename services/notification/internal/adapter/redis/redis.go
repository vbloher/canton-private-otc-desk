package redis

import (
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/vbloher/canton-private-otc-desk/pkg/redis"
)

const (
	cooldownKeyPrefix = "notification:confirmation:party:"
	partyKeyPrefix    = "notification:confirmation:party-day:"
	emailKeyPrefix    = "notification:confirmation:email:"
	totalKey          = "notification:confirmation:total"

	day = 24 * time.Hour

	partyCooldown = time.Minute
	partyPerDay   = 10
	emailPerDay   = 5
)

//nolint:gochecknoglobals
var countInWindow = goredis.NewScript(`
local count = redis.call('INCR', KEYS[1])
if count == 1 then
  redis.call('PEXPIRE', KEYS[1], ARGV[1])
end
return count
`)

type Config struct {
	DailyLimit int `env:"EMAIL_CONFIRMATIONS_DAILY_LIMIT" envDefault:"1000"`
}

type Limits struct {
	PartyCooldown time.Duration
	PartyPerDay   int
	EmailPerDay   int
	TotalPerDay   int
}

func LimitsFrom(c Config) Limits {
	return Limits{
		PartyCooldown: partyCooldown,
		PartyPerDay:   partyPerDay,
		EmailPerDay:   emailPerDay,
		TotalPerDay:   c.DailyLimit,
	}
}

type Redis struct {
	redis  *redis.Client
	limits Limits
}

func New(client *redis.Client, limits Limits) *Redis {
	return &Redis{redis: client, limits: limits}
}
