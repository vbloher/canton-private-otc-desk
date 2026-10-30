package usecase

import (
	"context"
	"errors"
	"fmt"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/dto"
)

func (u *UseCase) CompleteRegistration(
	ctx context.Context, input dto.CompleteRegistrationInput,
) (dto.CompleteRegistrationOutput, error) {
	ctx, span := tracer.Start(ctx, "usecase CompleteRegistration")
	defer span.End()

	var output dto.CompleteRegistrationOutput

	err := input.Validate()
	if err != nil {
		return output, fmt.Errorf("input.Validate: %w", err)
	}

	reg, err := u.redis.TakeRegistration(ctx, input.RegistrationID)
	if err != nil {
		return output, fmt.Errorf("redis.TakeRegistration: %w", err)
	}

	err = reg.VerifySignature(input.Signature)
	if err != nil {
		return output, fmt.Errorf("reg.VerifySignature: %w", err)
	}

	_, err = u.postgres.GetUserByCredential(ctx, input.CredentialID)

	switch {
	case err == nil:
		return output, domain.ErrCredentialTaken
	case !errors.Is(err, domain.ErrNotFound):
		return output, fmt.Errorf("postgres.GetUserByCredential: %w", err)
	}

	partyID, err := u.partyOf(ctx, reg, input.Signature)
	if err != nil {
		return output, err
	}

	user, err := reg.NewUser(partyID, input.CredentialID, input.EncryptedBlob)
	if err != nil {
		return output, fmt.Errorf("reg.NewUser: %w", err)
	}

	err = u.postgres.CreateUser(ctx, user)
	if err != nil {
		return output, fmt.Errorf("postgres.CreateUser: %w", err)
	}

	token, err := u.startSession(ctx, user.ID)
	if err != nil {
		return output, fmt.Errorf("u.startSession: %w", err)
	}

	return dto.CompleteRegistrationOutput{
		User:  user,
		Token: token,
		TTL:   u.config.SessionTTL,
	}, nil
}
