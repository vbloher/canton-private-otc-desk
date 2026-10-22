package usecase

import (
	"context"
	"fmt"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/dto"
)

// CreateChallenge issues the one-time text the wallet signs to connect, and hands over the wrapped
// key the browser needs to do it.
func (u *UseCase) CreateChallenge(
	ctx context.Context, input dto.CreateChallengeInput,
) (dto.CreateChallengeOutput, error) {
	ctx, span := tracer.Start(ctx, "usecase CreateChallenge")
	defer span.End()

	var output dto.CreateChallengeOutput

	user, err := u.postgres.GetUserByCredential(ctx, input.CredentialID)
	if err != nil {
		return output, fmt.Errorf("postgres.GetUserByCredential: %w", err)
	}

	challenge := domain.NewChallenge()

	err = u.redis.SaveChallenge(ctx, challenge, user.ID, challengeTTL)
	if err != nil {
		return output, fmt.Errorf("redis.SaveChallenge: %w", err)
	}

	return dto.CreateChallengeOutput{
		Challenge:     challenge,
		EncryptedBlob: user.EncryptedBlob,
		PublicKey:     user.PublicKey,
	}, nil
}
