package usecase_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/services/notification/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/notification/internal/dto"
	"github.com/vbloher/canton-private-otc-desk/services/notification/internal/usecase"
)

func preferences(news, trades bool) dto.SetPreferencesInput {
	return dto.SetPreferencesInput{PartyID: alice, News: news, Trades: trades}
}

func Test_SetPreferences_NeedsAConfirmedAddress(t *testing.T) {
	cases := map[string]struct {
		profile domain.Profile
		err     error
	}{
		"no profile":  {domain.Profile{}, domain.ErrNotFound},
		"unconfirmed": {domain.Profile{PartyID: alice, Email: "a@b.co"}, nil},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			d := newDeps(t)

			d.postgres.EXPECT().GetProfileForUpdate(inTx(), alice).Return(c.profile, c.err)

			_, err := d.useCase(t, testConfig).SetPreferences(t.Context(), preferences(true, true))

			require.ErrorIs(t, err, domain.ErrEmailNotConfirmed)
		})
	}
}

func Test_SetPreferences_ChangingNewsQueuesAContactSync(t *testing.T) {
	d := newDeps(t)
	profile := confirmedProfile(alice, "a@b.co")

	want := profile
	want.News = true
	want.Trades = true

	d.postgres.EXPECT().GetProfileForUpdate(inTx(), alice).Return(profile, nil)
	d.postgres.EXPECT().SaveProfile(inTx(), want).Return(nil)
	d.postgres.EXPECT().QueueContactSync(inTx(), "a@b.co", Any).Return(nil)
	d.events.EXPECT().Publish(Any, profileEvent(), []string{alice}).Return(nil)

	output, err := d.useCase(t, testConfig).SetPreferences(t.Context(), preferences(true, true))

	require.NoError(t, err)
	require.Equal(t, want, output.Profile)
}

func Test_SetPreferences_ChangingOnlyTradesLeavesResendAlone(t *testing.T) {
	d := newDeps(t)
	profile := confirmedProfile(alice, "a@b.co")

	want := profile
	want.Trades = true

	d.postgres.EXPECT().GetProfileForUpdate(inTx(), alice).Return(profile, nil)
	d.postgres.EXPECT().SaveProfile(inTx(), want).Return(nil)
	d.events.EXPECT().Publish(Any, profileEvent(), []string{alice}).Return(nil)

	_, err := d.useCase(t, testConfig).SetPreferences(t.Context(), preferences(false, true))

	require.NoError(t, err)
}

func Test_SetPreferences_RefusesWhenEmailIsOff(t *testing.T) {
	d := newDeps(t)

	_, err := d.useCase(t, usecase.Config{}).SetPreferences(t.Context(), preferences(true, true))

	require.ErrorIs(t, err, domain.ErrEmailDisabled)
}
