package redis

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog/log"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/domain"
)

func (r *Redis) GetTokenRules(ctx context.Context) (domain.Disclosed, bool) {
	ctx, span := tracer.Start(ctx, "adapter redis GetTokenRules")
	defer span.End()

	data, err := r.redis.Get(ctx, rulesKey).Bytes()
	if err != nil {
		if !errors.Is(err, redis.Nil) {
			log.Error().Err(err).Msg("redis: GetTokenRules: r.redis.Get")
		}

		return domain.Disclosed{}, false
	}

	var d disclosed

	err = json.Unmarshal(data, &d)
	if err != nil {
		log.Error().Err(err).Msg("redis: GetTokenRules: json.Unmarshal")

		return domain.Disclosed{}, false
	}

	if d.ContractID == "" {
		return domain.Disclosed{}, false
	}

	return domain.Disclosed{
		TemplateID:       d.TemplateID,
		ContractID:       d.ContractID,
		CreatedEventBlob: d.CreatedEventBlob,
		SynchronizerID:   d.SynchronizerID,
	}, true
}
