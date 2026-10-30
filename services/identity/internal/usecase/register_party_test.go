package usecase_test

import (
	"context"
	"crypto/ed25519"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/dto"
)

func Test_RegisterParty_AllocatesAndKeepsTheRegistration(t *testing.T) {
	d := newDeps(t)

	reg, signature := parkedRegistration(t)

	var saved domain.Registration

	d.redis.EXPECT().GetRegistration(Any, reg.ID).Return(reg, nil)
	d.ledger.EXPECT().AllocateExternalParty(Any, reg.Topology, signature).Return(reg.Topology.PartyID, nil)
	d.redis.EXPECT().SaveRegistration(Any, Any, 10*time.Minute).
		Run(func(_ context.Context, r domain.Registration, _ time.Duration) { saved = r }).
		Return(nil)

	output, err := d.useCase().RegisterParty(t.Context(), dto.RegisterPartyInput{RegistrationID: reg.ID, Signature: signature})
	require.NoError(t, err)
	require.Equal(t, reg.Topology.PartyID, output.PartyID)
	require.Equal(t, reg.Topology.PartyID, saved.PartyID)
	require.Equal(t, reg.ID, saved.ID)
}

func Test_RegisterParty_RepeatReturnsTheSameParty(t *testing.T) {
	d := newDeps(t)

	reg, signature := parkedRegistration(t)
	reg.PartyID = reg.Topology.PartyID

	d.redis.EXPECT().GetRegistration(Any, reg.ID).Return(reg, nil)

	output, err := d.useCase().RegisterParty(t.Context(), dto.RegisterPartyInput{RegistrationID: reg.ID, Signature: signature})
	require.NoError(t, err)
	require.Equal(t, reg.Topology.PartyID, output.PartyID)
}

func Test_RegisterParty_OnboardingPaused(t *testing.T) {
	d := newDeps(t)

	reg, signature := parkedRegistration(t)

	d.redis.EXPECT().GetRegistration(Any, reg.ID).Return(reg, nil)
	d.ledger.EXPECT().AllocateExternalParty(Any, reg.Topology, signature).Return("", domain.ErrOnboardingPaused)

	_, err := d.useCase().RegisterParty(t.Context(), dto.RegisterPartyInput{RegistrationID: reg.ID, Signature: signature})
	require.ErrorIs(t, err, domain.ErrOnboardingPaused)
}

func Test_RegisterParty_BadSignature(t *testing.T) {
	d := newDeps(t)

	reg, _ := parkedRegistration(t)

	_, otherKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	d.redis.EXPECT().GetRegistration(Any, reg.ID).Return(reg, nil)

	_, err = d.useCase().RegisterParty(t.Context(), dto.RegisterPartyInput{
		RegistrationID: reg.ID,
		Signature:      ed25519.Sign(otherKey, reg.Topology.MultiHash),
	})
	require.ErrorIs(t, err, domain.ErrValidation)
}

func Test_RegisterParty_Expired(t *testing.T) {
	d := newDeps(t)

	id := uuid.New()
	d.redis.EXPECT().GetRegistration(Any, id).Return(domain.Registration{}, domain.ErrRegistrationExpired)

	_, err := d.useCase().RegisterParty(t.Context(), dto.RegisterPartyInput{RegistrationID: id, Signature: make([]byte, 64)})
	require.ErrorIs(t, err, domain.ErrRegistrationExpired)
}
