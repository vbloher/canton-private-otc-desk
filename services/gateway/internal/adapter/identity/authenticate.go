package identity

import (
	"context"
	"fmt"

	pb "github.com/vbloher/canton-private-otc-desk/gen/grpc/identity_v1"
	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/pkg/principal"
)

func (i *Identity) Authenticate(ctx context.Context, sessionToken string) (principal.Principal, error) {
	ctx, span := tracer.Start(ctx, "adapter identity Authenticate")
	defer span.End()

	user, err := i.client.Authenticate(ctx, &pb.AuthenticateInput{SessionToken: sessionToken})
	if err != nil {
		return principal.Principal{}, fmt.Errorf("client.Authenticate: %w", mapError(err))
	}

	p, err := toPrincipal(user)
	if err != nil {
		return principal.Principal{}, fmt.Errorf("toPrincipal: %w", err)
	}

	return p, nil
}
