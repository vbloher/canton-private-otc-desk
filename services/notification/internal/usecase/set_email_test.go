package usecase_test

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/services/notification/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/notification/internal/dto"
	"github.com/vbloher/canton-private-otc-desk/services/notification/internal/usecase"
)

var errRedis = errors.New("redis is down")

func input(email string) dto.SetEmailInput {
	return dto.SetEmailInput{PartyID: alice, Email: email}
}

func (d deps) expectStored(profile domain.Profile, confirmation *domain.Confirmation, email *domain.Email) {
	d.postgres.EXPECT().SaveProfile(inTx(), profile).Return(nil)
	d.postgres.EXPECT().ReplaceConfirmation(inTx(), Any).
		Run(func(_ context.Context, c domain.Confirmation) { *confirmation = c }).Return(nil)
	d.postgres.EXPECT().CreateEmail(inTx(), Any).Run(func(_ context.Context, e domain.Email) { *email = e }).Return(nil)
}

func Test_SetEmail_StoresTheAddressTokenAndLetterInOneTransaction(t *testing.T) {
	d := newDeps(t)

	var (
		confirmation domain.Confirmation
		letter       domain.Email
	)

	d.postgres.EXPECT().GetProfile(Any, alice).Return(domain.Profile{}, domain.ErrNotFound)
	d.redis.EXPECT().AllowConfirmation(Any, alice, "a@b.co").Return(true, nil)
	d.postgres.EXPECT().GetProfileForUpdate(inTx(), alice).Return(domain.Profile{}, domain.ErrNotFound)
	d.expectStored(domain.Profile{PartyID: alice, Email: "a@b.co"}, &confirmation, &letter)

	output, err := d.useCase(t, testConfig).SetEmail(t.Context(), input(" A@B.co "))

	require.NoError(t, err)
	require.Equal(t, domain.Profile{PartyID: alice, Email: "a@b.co"}, output.Profile)
	require.True(t, output.EmailEnabled)

	require.Equal(t, alice, confirmation.PartyID)
	require.Equal(t, "a@b.co", confirmation.Email)
	require.Equal(t, "a@b.co", letter.Recipient)
	require.Equal(t, domain.KindConfirmation, letter.Kind)
	require.Equal(t, domain.DeliveryPending, letter.Status)

	link := regexp.MustCompile(regexp.QuoteMeta(publicURL) + `/confirm-email#token=([\w-]+)`).FindStringSubmatch(letter.Text)
	require.NotNil(t, link, letter.Text)
	require.Equal(t, confirmation.TokenHash, domain.HashToken(link[1]))
}

func Test_SetEmail_TheSameConfirmedAddressChangesNothing(t *testing.T) {
	d := newDeps(t)
	profile := confirmedProfile(alice, "a@b.co")

	d.postgres.EXPECT().GetProfile(Any, alice).Return(profile, nil)

	output, err := d.useCase(t, testConfig).SetEmail(t.Context(), input("a@b.co"))

	require.NoError(t, err)
	require.Equal(t, profile, output.Profile)
}

func Test_SetEmail_TheSameUnconfirmedAddressSendsAFreshLink(t *testing.T) {
	d := newDeps(t)
	profile := domain.Profile{PartyID: alice, Email: "a@b.co"}

	var (
		confirmation domain.Confirmation
		letter       domain.Email
	)

	d.postgres.EXPECT().GetProfile(Any, alice).Return(profile, nil)
	d.redis.EXPECT().AllowConfirmation(Any, alice, "a@b.co").Return(true, nil)
	d.postgres.EXPECT().GetProfileForUpdate(inTx(), alice).Return(profile, nil)
	d.expectStored(profile, &confirmation, &letter)

	_, err := d.useCase(t, testConfig).SetEmail(t.Context(), input("a@b.co"))

	require.NoError(t, err)
	require.Equal(t, "a@b.co", letter.Recipient)
}

func Test_SetEmail_ANewAddressDropsTheConfirmationAndUnsubscribesTheOldOne(t *testing.T) {
	d := newDeps(t)
	old := confirmedProfile(alice, "old@b.co")
	old.News = true
	old.Trades = true

	var (
		confirmation domain.Confirmation
		letter       domain.Email
	)

	d.postgres.EXPECT().GetProfile(Any, alice).Return(old, nil)
	d.redis.EXPECT().AllowConfirmation(Any, alice, "new@b.co").Return(true, nil)
	d.postgres.EXPECT().GetProfileForUpdate(inTx(), alice).Return(old, nil)
	d.expectStored(domain.Profile{PartyID: alice, Email: "new@b.co", News: true, Trades: true}, &confirmation, &letter)
	d.postgres.EXPECT().QueueContactSync(inTx(), "old@b.co", Any).Return(nil)

	output, err := d.useCase(t, testConfig).SetEmail(t.Context(), input("new@b.co"))

	require.NoError(t, err)
	require.Equal(t, domain.EmailUnconfirmed, output.Profile.Status())
	require.Equal(t, "new@b.co", letter.Recipient)
}

func Test_SetEmail_AnOldAddressWithoutNewsNeedsNoSync(t *testing.T) {
	d := newDeps(t)
	old := confirmedProfile(alice, "old@b.co")

	var (
		confirmation domain.Confirmation
		letter       domain.Email
	)

	d.postgres.EXPECT().GetProfile(Any, alice).Return(old, nil)
	d.redis.EXPECT().AllowConfirmation(Any, alice, "new@b.co").Return(true, nil)
	d.postgres.EXPECT().GetProfileForUpdate(inTx(), alice).Return(old, nil)
	d.expectStored(domain.Profile{PartyID: alice, Email: "new@b.co"}, &confirmation, &letter)

	_, err := d.useCase(t, testConfig).SetEmail(t.Context(), input("new@b.co"))

	require.NoError(t, err)
}

func Test_SetEmail_OverTheLimitWritesNothing(t *testing.T) {
	d := newDeps(t)

	d.postgres.EXPECT().GetProfile(Any, alice).Return(domain.Profile{}, domain.ErrNotFound)
	d.redis.EXPECT().AllowConfirmation(Any, alice, "a@b.co").Return(false, nil)

	_, err := d.useCase(t, testConfig).SetEmail(t.Context(), input("a@b.co"))

	require.ErrorIs(t, err, domain.ErrTooManyRequests)
}

func Test_SetEmail_ReportsAFailingLimiter(t *testing.T) {
	d := newDeps(t)

	d.postgres.EXPECT().GetProfile(Any, alice).Return(domain.Profile{}, domain.ErrNotFound)
	d.redis.EXPECT().AllowConfirmation(Any, alice, "a@b.co").Return(false, errRedis)

	_, err := d.useCase(t, testConfig).SetEmail(t.Context(), input("a@b.co"))

	require.ErrorIs(t, err, errRedis)
}

func Test_SetEmail_RejectsAnInvalidAddress(t *testing.T) {
	d := newDeps(t)

	_, err := d.useCase(t, testConfig).SetEmail(t.Context(), input("not an address"))

	require.ErrorIs(t, err, domain.ErrValidation)
}

func Test_SetEmail_RefusesWhenEmailIsOff(t *testing.T) {
	d := newDeps(t)

	_, err := d.useCase(t, usecase.Config{}).SetEmail(t.Context(), input("a@b.co"))

	require.ErrorIs(t, err, domain.ErrEmailDisabled)
}
