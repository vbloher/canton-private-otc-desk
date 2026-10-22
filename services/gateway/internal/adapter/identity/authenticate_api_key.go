package identity

import (
	"context"
	"fmt"

	pb "github.com/vbloher/canton-private-otc-desk/gen/grpc/identity_v1"
	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/pkg/principal"
)

func (i *Identity) AuthenticateAPIKey(ctx context.Context, apiKey string) (principal.Principal, error) {
	ctx, span := tracer.Start(ctx, "adapter identity AuthenticateAPIKey")
	defer span.End()

	user, err := i.client.AuthenticateAPIKey(ctx, &pb.AuthenticateAPIKeyInput{ApiKey: apiKey})
	if err != nil {
		return principal.Principal{}, fmt.Errorf("client.AuthenticateAPIKey: %w", mapError(err))
	}

	p, err := toPrincipal(user)
	if err != nil {
		return principal.Principal{}, fmt.Errorf("toPrincipal: %w", err)
	}

	return p, nil
}
