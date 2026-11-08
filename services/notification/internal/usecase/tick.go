package usecase

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/notification/internal/domain"
)

const (
	batchSize      = 50
	cleanEvery     = time.Hour
	emailRetention = 30 * 24 * time.Hour

	statusSent    = "sent"
	statusRetried = "retried"
	statusFailed  = "failed"
)

func (u *UseCase) Tick(ctx context.Context, now time.Time) error {
	if !u.config.Enabled {
		return nil
	}

	return errors.Join(u.clean(ctx, now), u.deliver(ctx, now), u.syncContacts(ctx, now))
}

func (u *UseCase) clean(ctx context.Context, now time.Time) error {
	if now.Sub(u.cleaned) < cleanEvery {
		return nil
	}

	err := u.postgres.Cleanup(ctx, now, now.Add(-emailRetention))
	if err != nil {
		return fmt.Errorf("postgres.Cleanup: %w", err)
	}

	u.cleaned = now

	return nil
}

func (u *UseCase) deliver(ctx context.Context, now time.Time) error {
	for range batchSize {
		more, err := u.deliverOne(ctx, now)
		if err != nil {
			return err
		}

		if !more {
			return nil
		}
	}

	return nil
}

func (u *UseCase) deliverOne(ctx context.Context, now time.Time) (bool, error) {
	var more bool

	err := u.tx.Wrap(ctx, func(ctx context.Context) error {
		email, err := u.postgres.NextDueEmail(ctx, now)
		if errors.Is(err, domain.ErrNotFound) {
			return nil
		}

		if err != nil {
			return fmt.Errorf("postgres.NextDueEmail: %w", err)
		}

		updated, keepGoing := u.attempt(ctx, email, now)
		more = keepGoing

		err = u.postgres.UpdateEmail(ctx, updated)
		if err != nil {
			return fmt.Errorf("postgres.UpdateEmail: %w", err)
		}

		return nil
	})
	if err != nil {
		return false, fmt.Errorf("transaction.Wrap: %w", err)
	}

	return more, nil
}

func (u *UseCase) attempt(ctx context.Context, e domain.Email, now time.Time) (domain.Email, bool) {
	ctx, span := tracer.StartRoot(ctx, "usecase SendEmail")
	defer span.End()

	kind := string(e.Kind)

	providerID, err := u.sender.Send(ctx, e)

	switch {
	case err == nil:
		emailsTotal.WithLabelValues(kind, statusSent).Inc()

		return e.Delivered(providerID, now), true
	case errors.Is(err, domain.ErrUndeliverable):
		emailsTotal.WithLabelValues(kind, statusFailed).Inc()
		log.Error().Ctx(ctx).Err(err).Stringer("email", e.ID).Str("kind", kind).Msg("usecase: email rejected")

		return e.Undeliverable(err.Error(), now), true
	}

	retried := e.Retry(err.Error(), now)
	status := statusRetried

	if retried.Status == domain.DeliveryFailed {
		status = statusFailed
	}

	emailsTotal.WithLabelValues(kind, status).Inc()
	log.Warn().Ctx(ctx).Err(err).Stringer("email", e.ID).Str("kind", kind).Int("attempts", retried.Attempts).
		Msg("usecase: email not sent")

	return retried, false
}

func (u *UseCase) syncContacts(ctx context.Context, now time.Time) error {
	for range batchSize {
		more, err := u.syncOne(ctx, now)
		if err != nil {
			return err
		}

		if !more {
			return nil
		}
	}

	return nil
}

func (u *UseCase) syncOne(ctx context.Context, now time.Time) (bool, error) {
	var more bool

	err := u.tx.Wrap(ctx, func(ctx context.Context) error {
		sync, err := u.postgres.NextDueContactSync(ctx, now)
		if errors.Is(err, domain.ErrNotFound) {
			return nil
		}

		if err != nil {
			return fmt.Errorf("postgres.NextDueContactSync: %w", err)
		}

		more, err = u.pushContact(ctx, sync, now)

		return err
	})
	if err != nil {
		return false, fmt.Errorf("transaction.Wrap: %w", err)
	}

	return more, nil
}

func (u *UseCase) pushContact(ctx context.Context, sync domain.ContactSync, now time.Time) (bool, error) {
	ctx, span := tracer.StartRoot(ctx, "usecase SyncContact")
	defer span.End()

	wanted, err := u.postgres.NewsWanted(ctx, sync.Email)
	if err != nil {
		return false, fmt.Errorf("postgres.NewsWanted: %w", err)
	}

	push := u.contacts.Unsubscribe
	if wanted {
		push = u.contacts.Subscribe
	}

	failure := push(ctx, sync.Email)
	if failure == nil {
		return true, u.dropContactSync(ctx, sync.Email)
	}

	next := sync.Retry(now)

	if next.Exhausted() {
		log.Error().Ctx(ctx).Err(failure).Msg("usecase: contact sync abandoned")

		return true, u.dropContactSync(ctx, sync.Email)
	}

	log.Warn().Ctx(ctx).Err(failure).Int("attempts", next.Attempts).Msg("usecase: contact sync failed")

	err = u.postgres.RescheduleContactSync(ctx, next)
	if err != nil {
		return false, fmt.Errorf("postgres.RescheduleContactSync: %w", err)
	}

	return false, nil
}

func (u *UseCase) dropContactSync(ctx context.Context, email string) error {
	err := u.postgres.DeleteContactSync(ctx, email)
	if err != nil {
		return fmt.Errorf("postgres.DeleteContactSync: %w", err)
	}

	return nil
}
