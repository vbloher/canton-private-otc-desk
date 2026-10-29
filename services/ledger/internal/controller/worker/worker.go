package worker

import (
	"context"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/usecase"
)

const interval = 30 * time.Second

type Worker struct {
	usecase *usecase.UseCase
	stop    context.CancelFunc
	done    chan struct{}
}

func New(uc *usecase.UseCase) *Worker {
	ctx, stop := context.WithCancel(context.Background())

	w := &Worker{
		usecase: uc,
		stop:    stop,
		done:    make(chan struct{}),
	}

	go w.run(tracer.Suppress(ctx))

	return w
}

func (w *Worker) run(ctx context.Context) {
	defer close(w.done)

	log.Info().Msg("expire worker: started")

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.expire(ctx)
		}
	}
}

func (w *Worker) expire(ctx context.Context) {
	output, err := w.usecase.ExpireActions(ctx)
	if err != nil {
		log.Error().Err(err).Msg("expire worker: usecase.ExpireActions")

		return
	}

	if output.Expired > 0 {
		log.Info().Int("expired", output.Expired).Msg("expire worker: actions expired")
	}
}

func (w *Worker) Close() {
	log.Info().Msg("expire worker: closing")

	w.stop()

	<-w.done

	log.Info().Msg("expire worker: closed")
}
