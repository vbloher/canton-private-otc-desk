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

// Init waits for the ledger and its seeded parties (/readyz turns green before the seed
// script has run), resolves the operator and the house parties, and refreshes the caches
// so a re-created sandbox cannot be served stale contract ids. It gives up after
// StartupTimeout.
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

	ids, err := u.resolveParties(ctx)
	if err != nil {
		return err
	}

	u.operator = ids[u.config.OperatorHint]

	for _, id := range ids {
		u.hosted[id] = true
	}

	log.Info().Str("operator", u.operator).Strs("house", u.config.HouseParties).Msg("ledger: parties resolved")

	return u.warmUp(ctx)
}

func (u *UseCase) resolveParties(ctx context.Context) (map[string]string, error) {
	hints := append([]string{u.config.OperatorHint}, u.config.HouseParties...)
	ids := make(map[string]string, len(hints))

	var missing []string

	err := u.poll(ctx, func(ctx context.Context) (bool, error) {
		missing = missing[:0]

		for _, hint := range hints {
			if ids[hint] != "" {
				continue
			}

			id, err := u.resolveHint(ctx, hint)
			if err != nil {
				return false, err
			}

			if id == "" {
				missing = append(missing, hint)

				continue
			}

			ids[hint] = id
		}

		if len(missing) > 0 {
			log.Info().Strs("missing", missing).Msg("ledger: waiting for parties")

			return false, nil
		}

		return true, nil
	})
	if errors.Is(err, domain.ErrAmbiguousParty) {
		return nil, err
	}

	if err != nil {
		return nil, fmt.Errorf("parties not found on the ledger: %s (is it seeded?): %w",
			strings.Join(missing, ", "), err)
	}

	return ids, nil
}

// resolveHint is the party's id, or "" while it is not there yet. Only an ambiguous hint (a
// ledger seeded twice) is an error: waiting cannot fix that.
func (u *UseCase) resolveHint(ctx context.Context, hint string) (string, error) {
	id, err := u.canton.ResolveParty(ctx, hint)

	switch {
	case err == nil:
		return id, nil
	case errors.Is(err, domain.ErrAmbiguousParty):
		return "", fmt.Errorf("canton.ResolveParty: %w", err)
	case !errors.Is(err, domain.ErrNotFound):
		log.Warn().Err(err).Str("hint", hint).Msg("ledger: canton.ResolveParty")
	}

	return "", nil
}

// warmUp caches the TokenRules (the seed creates them right after the parties) and the
// synchronizer.
func (u *UseCase) warmUp(ctx context.Context) error {
	var (
		rules   domain.Disclosed
		lastErr error
	)

	err := u.poll(ctx, func(ctx context.Context) (bool, error) {
		rules, lastErr = u.canton.TokenRules(ctx, u.operator)
		if lastErr != nil {
			log.Info().Err(lastErr).Msg("ledger: waiting for TokenRules")

			return false, nil //nolint:nilerr // the seed creates them right after the parties: wait
		}

		return true, nil
	})
	if err != nil {
		return fmt.Errorf("canton.TokenRules: %w", errors.Join(err, lastErr))
	}

	u.redis.SetTokenRules(ctx, rules)

	id, err := u.canton.SynchronizerID(ctx, u.operator)
	if err != nil {
		return fmt.Errorf("canton.SynchronizerID: %w", err)
	}

	u.redis.SetSynchronizerID(ctx, id)

	return nil
}

// poll calls done every StartupInterval until it reports true, fails, or ctx ends.
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
