package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/domain"
)

func (u *UseCase) Init(ctx context.Context) error {
	ctx, span := tracer.Start(ctx, "usecase Init")
	defer span.End()

	ctx, cancel := context.WithTimeout(ctx, u.config.startupTimeout())
	defer cancel()

	err := u.poll(ctx, func(ctx context.Context) (bool, error) {
		if u.canton.Ready(ctx) {
			return true, nil
		}

		log.Info().Msg("ledger: waiting for the ledger to be ready")

		return false, nil
	})
	if err != nil {
		return fmt.Errorf("the ledger is not ready: %w", err)
	}

	err = u.awaitParties(ctx)
	if err != nil {
		return err
	}

	u.operator = u.config.OperatorParty

	for _, party := range u.parties() {
		u.hosted[party] = true
	}

	if u.config.faucetEnabled() {
		u.faucet = u.config.FaucetParty
	} else if u.config.FaucetParty != "" {
		log.Warn().Str("network", string(u.config.Network)).
			Msg("ledger: the faucet needs CANTON_NETWORK=devnet and FAUCET_AMOUNTS, it stays off")
	}

	log.Info().Str("network", string(u.config.Network)).Str("operator", u.operator).
		Str("maker", u.config.MakerParty).Str("faucet", u.faucet).Msg("ledger: parties found")

	return u.warmUp(ctx)
}

func (u *UseCase) parties() []string {
	parties := []string{u.config.OperatorParty}

	if u.config.MakerParty != "" {
		parties = append(parties, u.config.MakerParty)
	}

	if u.config.faucetEnabled() {
		parties = append(parties, u.config.FaucetParty)
	}

	return parties
}

func (u *UseCase) awaitParties(ctx context.Context) error {
	parties := u.parties()

	for _, party := range parties {
		if !strings.Contains(party, "::") {
			return domain.Errorf(domain.ErrValidation, "%q is not a full party id", party)
		}
	}

	found := make(map[string]bool, len(parties))

	var missing []string

	err := u.poll(ctx, func(ctx context.Context) (bool, error) {
		missing = missing[:0]

		for _, party := range parties {
			if found[party] {
				continue
			}

			if u.onLedger(ctx, party) {
				found[party] = true

				continue
			}

			missing = append(missing, party)
		}

		if len(missing) > 0 {
			log.Info().Strs("missing", missing).Msg("ledger: waiting for parties")

			return false, nil
		}

		return true, nil
	})
	if err != nil {
		return fmt.Errorf("parties not on the participant: %s: %w", strings.Join(missing, ", "), err)
	}

	return nil
}

func (u *UseCase) onLedger(ctx context.Context, party string) bool {
	id, err := u.canton.ResolveParty(ctx, party)
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		log.Warn().Err(err).Str("party", party).Msg("ledger: canton.ResolveParty")
	}

	return err == nil && id == party
}

func (u *UseCase) warmUp(ctx context.Context) error {
	id, err := u.canton.SynchronizerID(ctx, u.operator)
	if err != nil {
		return fmt.Errorf("canton.SynchronizerID: %w", err)
	}

	u.redis.SetSynchronizerID(ctx, id)

	return nil
}

func (u *UseCase) poll(ctx context.Context, done func(ctx context.Context) (bool, error)) error {
	ticker := time.NewTicker(u.config.startupInterval())
	defer ticker.Stop()

	for {
		ok, err := done(ctx)
		if err != nil || ok {
			return err
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("gave up waiting: %w", ctx.Err())
		case <-ticker.C:
		}
	}
}
