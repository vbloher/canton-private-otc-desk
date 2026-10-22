// Package identity is the trading service's client of the identity service (IdentityV1 over gRPC).
package identity

import (
	"fmt"
	"time"

	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"

	"github.com/vbloher/canton-private-otc-desk/gen/grpc/identity_v1"
	"github.com/vbloher/canton-private-otc-desk/pkg/grpcclient"
)

const callTimeout = 30 * time.Second

type Config struct {
	Addr string `env:"IDENTITY_GRPC_ADDR" envDefault:"localhost:9081"`
}

type Identity struct {
	client identity_v1.IdentityV1Client
	conn   *grpc.ClientConn
}

// New connects lazily: the service starts while identity is down.
func New(c Config) (*Identity, error) {
	conn, err := grpcclient.New(c.Addr, callTimeout)
	if err != nil {
		return nil, fmt.Errorf("grpcclient.New: %w", err)
	}

	return &Identity{client: identity_v1.NewIdentityV1Client(conn), conn: conn}, nil
}

func (i *Identity) Close() {
	err := i.conn.Close()
	if err != nil {
		log.Error().Err(err).Msg("identity client: i.conn.Close")
	}
}
