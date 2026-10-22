package usecase

import (
	"context"

	"github.com/vbloher/canton-private-otc-desk/pkg/principal"
)

//go:generate go tool mockery --config=../../.mockery.yml

// Identity resolves credentials into the caller.
// Credentials it does not know are reported as domain.ErrUnauthorized.
type Identity interface {
	Authenticate(ctx context.Context, sessionToken string) (principal.Principal, error)
	AuthenticateAPIKey(ctx context.Context, apiKey string) (principal.Principal, error)
}

type UseCase struct {
	identity Identity
}

func New(identity Identity) *UseCase {
	return &UseCase{
		identity: identity,
	}
}
