package redis

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/rs/zerolog/log"

	goredis "github.com/redis/go-redis/v9"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/trading/internal/domain"
)

// GetDesk reports a miss on any failure: the caller then reads the ledger.
func (r *Redis) GetDesk(ctx context.Context) (domain.Desk, bool) {
	ctx, span := tracer.Start(ctx, "adapter redis GetDesk")
	defer span.End()

	b, err := r.redis.Get(ctx, deskKey).Bytes()
	if err != nil {
		if !errors.Is(err, goredis.Nil) {
			log.Error().Err(err).Msg("redis: GetDesk: r.redis.Get")
		}

		return domain.Desk{}, false
	}

	var d desk

	err = json.Unmarshal(b, &d)
	if err != nil {
		log.Error().Err(err).Msg("redis: GetDesk: json.Unmarshal")

		return domain.Desk{}, false
	}

	return domain.Desk{Operator: d.Operator, Instruments: d.Instruments}, true
}
