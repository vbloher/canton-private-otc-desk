package redis

import (
	"context"
	"encoding/json"

	"github.com/rs/zerolog/log"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/trading/internal/domain"
)

func (r *Redis) SetDesk(ctx context.Context, d domain.Desk) {
	ctx, span := tracer.Start(ctx, "adapter redis SetDesk")
	defer span.End()

	b, err := json.Marshal(desk{Operator: d.Operator, Instruments: d.Instruments})
	if err != nil {
		log.Error().Err(err).Msg("redis: SetDesk: json.Marshal")

		return
	}

	err = r.redis.Set(ctx, deskKey, b, deskTTL).Err()
	if err != nil {
		log.Error().Err(err).Msg("redis: SetDesk: r.redis.Set")
	}
}
