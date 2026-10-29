package app

import (
	"context"
	"fmt"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/vbloher/canton-private-otc-desk/pkg/events"
	"github.com/vbloher/canton-private-otc-desk/pkg/grpcclient"
	"github.com/vbloher/canton-private-otc-desk/pkg/httpserver"
	"github.com/vbloher/canton-private-otc-desk/pkg/metrics"
	redislib "github.com/vbloher/canton-private-otc-desk/pkg/redis"
	"github.com/vbloher/canton-private-otc-desk/pkg/router"
	"github.com/vbloher/canton-private-otc-desk/services/gateway/config"
	"github.com/vbloher/canton-private-otc-desk/services/gateway/internal/adapter/identity"
	"github.com/vbloher/canton-private-otc-desk/services/gateway/internal/controller/http"
	"github.com/vbloher/canton-private-otc-desk/services/gateway/internal/controller/ws"
	"github.com/vbloher/canton-private-otc-desk/services/gateway/internal/usecase"
)

// identityTimeout bounds one authentication call. It runs before every request, so it must not hang.
const identityTimeout = 5 * time.Second

func Run(ctx context.Context, c config.Config) error {
	redisClient, err := redislib.New(ctx, c.Redis)
	if err != nil {
		return fmt.Errorf("redislib.New: %w", err)
	}
	defer redisClient.Close()

	// The connection is lazy: the gateway starts without identity and answers 502 until it is up.
	identityConn, err := grpcclient.New(c.Identity.Addr, identityTimeout)
	if err != nil {
		return fmt.Errorf("grpcclient.New: %w", err)
	}

	defer func() {
		err := identityConn.Close()
		if err != nil {
			log.Error().Err(err).Msg("identity grpc client: close")
		}
	}()

	uc := usecase.New(identity.New(identityConn))

	hub := ws.NewHub(ws.PingInterval)
	defer hub.Close()

	stopFeed := feedHub(ctx, events.NewSubscriber(redisClient), hub)
	defer stopFeed()

	r := router.New()

	err = http.GatewayRouter(r, uc, hub, c.Upstreams, metrics.NewHTTPServer())
	if err != nil {
		return fmt.Errorf("http.GatewayRouter: %w", err)
	}

	httpServer, err := httpserver.New(ctx, r, c.HTTP)
	if err != nil {
		return fmt.Errorf("httpserver.New: %w", err)
	}
	defer httpServer.Close()

	log.Info().Msg("app: started")

	<-ctx.Done()

	log.Info().Msg("app: got signal to stop")

	return nil
}

// feedHub runs the one events subscriber of the process and hands every Redis payload to the hub.
// The returned function stops the subscriber and waits for it.
func feedHub(ctx context.Context, subscriber *events.Subscriber, hub *ws.Hub) (stop func()) {
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})

	go func() {
		defer close(done)

		subscriber.Run(ctx, hub.Deliver)
	}()

	return func() {
		cancel()
		<-done
	}
}
