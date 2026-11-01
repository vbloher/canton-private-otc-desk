package usecase

import (
	"context"
	"fmt"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/dto"
)

func (u *UseCase) CreateChallenge(
	ctx context.Context, input dto.CreateChallengeInput,
) (dto.CreateChallengeOutput, error) {
	ctx, span := tracer.Start(ctx, "usecase CreateChallenge")
	defer span.End()

	var output dto.CreateChallengeOutput

	err := input.Validate()
	if err != nil {
		return output, fmt.Errorf("input.Validate: %w", err)
	}

	user, err := u.challenged(ctx, input)
	if err != nil {
		return output, err
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

func (u *UseCase) challenged(ctx context.Context, input dto.CreateChallengeInput) (domain.User, error) {
	if input.PartyID == "" {
		user, err := u.postgres.GetUserByCredential(ctx, input.CredentialID)
		if err != nil {
			return domain.User{}, fmt.Errorf("postgres.GetUserByCredential: %w", err)
		}

		return user, nil
	}

	user, err := u.postgres.GetUserByParty(ctx, input.PartyID)
	if err != nil {
		return domain.User{}, fmt.Errorf("postgres.GetUserByParty: %w", err)
	}

	if user.Kind != domain.KindKey {
		return domain.User{}, fmt.Errorf("%s is not a key account: %w", input.PartyID, domain.ErrNotFound)
	}

	return user, nil
}
