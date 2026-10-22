package redis

import (
	"context"
	"encoding/json"

	"github.com/rs/zerolog/log"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/domain"
)

func (r *Redis) SetTokenRules(ctx context.Context, rules domain.Disclosed) {
	ctx, span := tracer.Start(ctx, "adapter redis SetTokenRules")
	defer span.End()

	data, err := json.Marshal(disclosed{
		TemplateID:       rules.TemplateID,
		ContractID:       rules.ContractID,
		CreatedEventBlob: rules.CreatedEventBlob,
		SynchronizerID:   rules.SynchronizerID,
	})
	if err != nil {
		log.Error().Err(err).Msg("redis: SetTokenRules: json.Marshal")

		return
	}

	err = r.redis.Set(ctx, rulesKey, data, ttl).Err()
	if err != nil {
		log.Error().Err(err).Msg("redis: SetTokenRules: r.redis.Set")
	}
}
