// Package ledger is the trading service's client of the ledger service (LedgerV1 over gRPC).
package ledger

import (
	"fmt"
	"time"

	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"

	"github.com/vbloher/canton-private-otc-desk/gen/grpc/ledger_v1"
	"github.com/vbloher/canton-private-otc-desk/pkg/grpcclient"
)

const callTimeout = 30 * time.Second

type Config struct {
	Addr string `env:"LEDGER_GRPC_ADDR" envDefault:"localhost:9083"`
}

type Ledger struct {
	client ledger_v1.LedgerV1Client
	conn   *grpc.ClientConn
}

// New connects lazily: the service starts while the ledger is down.
func New(c Config) (*Ledger, error) {
	conn, err := grpcclient.New(c.Addr, callTimeout)
	if err != nil {
		return nil, fmt.Errorf("grpcclient.New: %w", err)
	}

	return &Ledger{client: ledger_v1.NewLedgerV1Client(conn), conn: conn}, nil
}

func (l *Ledger) Close() {
	err := l.conn.Close()
	if err != nil {
		log.Error().Err(err).Msg("ledger client: l.conn.Close")
	}
}
