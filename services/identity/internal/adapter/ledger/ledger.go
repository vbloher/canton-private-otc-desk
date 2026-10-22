package ledger

import (
	"fmt"
	"time"

	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/vbloher/canton-private-otc-desk/gen/grpc/ledger_v1"
	"github.com/vbloher/canton-private-otc-desk/pkg/grpcclient"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/domain"
)

const callTimeout = 30 * time.Second

type Config struct {
	Addr string `env:"LEDGER_GRPC_ADDR" envDefault:"localhost:9083"`
}

// Ledger is the client of the ledger service. Connecting is lazy: identity starts while the ledger is down.
type Ledger struct {
	client pb.LedgerV1Client
	conn   *grpc.ClientConn
}

func New(c Config) (*Ledger, error) {
	conn, err := grpcclient.New(c.Addr, callTimeout)
	if err != nil {
		return nil, fmt.Errorf("grpcclient.New: %w", err)
	}

	return &Ledger{client: pb.NewLedgerV1Client(conn), conn: conn}, nil
}

func (l *Ledger) Close() {
	err := l.conn.Close()
	if err != nil {
		log.Error().Err(err).Msg("ledger client: l.conn.Close")
	}

	log.Info().Msg("ledger client: closed")
}

// mapError turns a failed call into a domain error: an unknown party is ErrNotFound, anything else
// is a ledger failure. The gRPC error stays in the chain for the logs.
func mapError(err error) error {
	if status.Code(err) == codes.NotFound {
		return fmt.Errorf("%w: %w", domain.ErrNotFound, err)
	}

	return fmt.Errorf("%w: %w", domain.ErrLedger, err)
}
