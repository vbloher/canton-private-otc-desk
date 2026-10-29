package identity

import (
	"google.golang.org/grpc"

	pb "github.com/vbloher/canton-private-otc-desk/gen/grpc/identity_v1"
)

type Config struct {
	Addr string `env:"IDENTITY_GRPC_ADDR" envDefault:"localhost:9081"`
}

type Identity struct {
	client pb.IdentityV1Client
}

func New(conn grpc.ClientConnInterface) *Identity {
	return &Identity{
		client: pb.NewIdentityV1Client(conn),
	}
}
