package usecase_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/services/notification/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/notification/internal/dto"
	"github.com/vbloher/canton-private-otc-desk/services/notification/internal/usecase"
)

func Test_GetProfile_ReturnsTheStoredProfile(t *testing.T) {
	d := newDeps(t)
	profile := confirmedProfile(alice, "a@b.co")

	d.postgres.EXPECT().GetProfile(Any, alice).Return(profile, nil)

	output, err := d.useCase(t, testConfig).GetProfile(t.Context(), dto.GetProfileInput{PartyID: alice})

	require.NoError(t, err)
	require.Equal(t, dto.ProfileOutput{Profile: profile, EmailEnabled: true}, output)
}

func Test_GetProfile_AWalletWithoutAProfileHasAnEmptyOne(t *testing.T) {
	d := newDeps(t)

	d.postgres.EXPECT().GetProfile(Any, alice).Return(domain.Profile{}, domain.ErrNotFound)

	output, err := d.useCase(t, testConfig).GetProfile(t.Context(), dto.GetProfileInput{PartyID: alice})

	require.NoError(t, err)
	require.Equal(t, domain.Profile{PartyID: alice}, output.Profile)
}

func Test_GetProfile_SaysWhenEmailIsOff(t *testing.T) {
	d := newDeps(t)

	d.postgres.EXPECT().GetProfile(Any, alice).Return(domain.Profile{}, domain.ErrNotFound)

	output, err := d.useCase(t, usecase.Config{}).GetProfile(t.Context(), dto.GetProfileInput{PartyID: alice})

	require.NoError(t, err)
	require.False(t, output.EmailEnabled)
}
