package usecase

import (
	"context"

	"github.com/vbloher/canton-private-otc-desk/pkg/principal"
)

//go:generate go tool mockery --config=../../.mockery.yml

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
