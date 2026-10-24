package usecase

import (
	"context"
	"errors"
	"fmt"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/dto"
)

// CompleteRegistration allocates the party with the browser key's signature, registers the wallet
// and signs it in. The parked registration is consumed by the first attempt that gets that far.
// Everything that can be checked is checked before the ledger is called: a party allocated for a
// registration that then fails would stay behind on the ledger for good.
func (u *UseCase) CompleteRegistration(
	ctx context.Context, input dto.CompleteRegistrationInput,
) (dto.CompleteRegistrationOutput, error) {
	ctx, span := tracer.Start(ctx, "usecase CompleteRegistration")
	defer span.End()

	var output dto.CompleteRegistrationOutput

	// A malformed request is refused before anything is touched: it does not burn the parked
	// registration and it cannot reach the ledger.
	err := input.Validate()
	if err != nil {
		return output, fmt.Errorf("input.Validate: %w", err)
	}

	reg, err := u.redis.TakeRegistration(ctx, input.RegistrationID)
	if err != nil {
		return output, fmt.Errorf("redis.TakeRegistration: %w", err)
	}

	// The signature and the credential id are checked against what is parked and stored before the
	// ledger is called: allocating the party first would leave an orphaned party behind on every clash.
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

	partyID, err := u.ledger.AllocateExternalParty(ctx, reg.Topology, input.Signature)
	if err != nil {
		return output, fmt.Errorf("ledger.AllocateExternalParty: %w", err)
	}

	// Nothing here is new to the registration: the input was validated above and the parked
	// registration was validated when it was started.
	user, err := reg.NewUser(partyID, input.CredentialID, input.EncryptedBlob)
	if err != nil {
		return output, fmt.Errorf("reg.NewUser: %w", err)
	}

	err = u.postgres.CreateUser(ctx, user)
	if err != nil {
		return output, fmt.Errorf("postgres.CreateUser: %w", err)
	}

	token, expiresAt, err := u.startSession(ctx, user.ID)
	if err != nil {
		return output, fmt.Errorf("u.startSession: %w", err)
	}

	return dto.CompleteRegistrationOutput{
		User:      user,
		Token:     token,
		TTL:       u.config.SessionTTL,
		ExpiresAt: expiresAt,
	}, nil
}
