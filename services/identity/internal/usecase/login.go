package usecase

import (
	"context"
	"errors"
	"fmt"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/dto"
)

// Login connects a wallet that signed its challenge. The challenge is consumed by the attempt,
// right or wrong: a failed signature needs a new challenge.
func (u *UseCase) Login(ctx context.Context, input dto.LoginInput) (dto.LoginOutput, error) {
	ctx, span := tracer.Start(ctx, "usecase Login")
	defer span.End()

	var output dto.LoginOutput

	err := domain.ValidateSignature(input.Signature)
	if err != nil {
		return output, fmt.Errorf("domain.ValidateSignature: %w", err)
	}

	userID, err := u.redis.TakeChallenge(ctx, input.Challenge)
	if err != nil {
		return output, fmt.Errorf("redis.TakeChallenge: %w", err)
	}

	user, err := u.postgres.GetUser(ctx, userID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return output, fmt.Errorf("the wallet of the challenge is gone: %w", domain.ErrUnauthorized)
		}

		return output, fmt.Errorf("postgres.GetUser: %w", err)
	}

	if user.CredentialID != input.CredentialID {
		return output, fmt.Errorf("the challenge belongs to another credential: %w", domain.ErrUnauthorized)
	}

	if !domain.VerifySignature(user.PublicKey, []byte(input.Challenge), input.Signature) {
		return output, fmt.Errorf("the signature does not match the challenge: %w", domain.ErrUnauthorized)
	}

	token, err := u.startSession(ctx, user.ID)
	if err != nil {
		return output, fmt.Errorf("u.startSession: %w", err)
	}

	return dto.LoginOutput{
		User:  user,
		Token: token,
		TTL:   u.config.SessionTTL,
	}, nil
}
