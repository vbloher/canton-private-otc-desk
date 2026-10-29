package worker

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/trading/internal/usecase"
)

var ErrInvalidConfig = errors.New("worker: interval must be positive")

type Config struct {
	Interval time.Duration `env:"WORKER_INTERVAL" envDefault:"1s"`
}

type Worker struct {
	config  Config
	usecase *usecase.UseCase
	stop    context.CancelFunc
	done    chan struct{}
}

func New(c Config, uc *usecase.UseCase) (*Worker, error) {
	if c.Interval <= 0 {
		return nil, fmt.Errorf("%w: %+v", ErrInvalidConfig, c)
	}

	ctx, stop := context.WithCancel(context.Background())

	w := &Worker{
		config:  c,
		usecase: uc,
		stop:    stop,
		done:    make(chan struct{}),
	}

	go w.run(tracer.Suppress(ctx))

	return w, nil
}

func (w *Worker) run(ctx context.Context) {
	defer close(w.done)

	log.Info().Msg("worker: started")

	ticker := time.NewTicker(w.config.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			err := w.usecase.Tick(ctx, time.Now())
			if err != nil {
				log.Error().Err(err).Msg("worker: usecase.Tick")
			}
		}
	}
}

func (w *Worker) Close() {
	log.Info().Msg("worker: closing")

	w.stop()

	<-w.done

	log.Info().Msg("worker: closed")
}
