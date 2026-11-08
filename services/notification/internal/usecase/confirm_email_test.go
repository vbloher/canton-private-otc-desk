package usecase_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/services/notification/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/notification/internal/dto"
)

const token = "the-token"

func pendingConfirmation(expires time.Time) domain.Confirmation {
	return domain.Confirmation{TokenHash: domain.HashToken(token), PartyID: alice, Email: "a@b.co", ExpiresAt: expires}
}

func (d deps) expectLookup(c domain.Confirmation, profile domain.Profile) {
	d.postgres.EXPECT().GetConfirmation(Any, c.TokenHash).Return(c, nil)
	d.postgres.EXPECT().GetProfileForUpdate(inTx(), c.PartyID).Return(profile, nil)
}

func Test_ConfirmEmail_ConfirmsTheAddressAndTellsTheOpenTab(t *testing.T) {
	d := newDeps(t)
	c := pendingConfirmation(time.Now().Add(time.Hour))

	var saved domain.Profile

	d.expectLookup(c, domain.Profile{PartyID: alice, Email: "a@b.co"})
	d.postgres.EXPECT().MarkConfirmed(inTx(), c.TokenHash, Any).Return(nil)
	d.postgres.EXPECT().SaveProfile(inTx(), Any).Run(func(_ context.Context, p domain.Profile) { saved = p }).Return(nil)
	d.events.EXPECT().Publish(Any, profileEvent(), []string{alice}).Return(nil)

	require.NoError(t, d.useCase(t, testConfig).ConfirmEmail(t.Context(), dto.ConfirmEmailInput{Token: token}))
	require.True(t, saved.Confirmed())
}

func Test_ConfirmEmail_SubscribesAnAddressThatAlreadyAskedForNews(t *testing.T) {
	d := newDeps(t)
	c := pendingConfirmation(time.Now().Add(time.Hour))

	d.expectLookup(c, domain.Profile{PartyID: alice, Email: "a@b.co", News: true})
	d.postgres.EXPECT().MarkConfirmed(inTx(), c.TokenHash, Any).Return(nil)
	d.postgres.EXPECT().SaveProfile(inTx(), Any).Return(nil)
	d.postgres.EXPECT().QueueContactSync(inTx(), "a@b.co", Any).Return(nil)
	d.events.EXPECT().Publish(Any, profileEvent(), []string{alice}).Return(nil)

	require.NoError(t, d.useCase(t, testConfig).ConfirmEmail(t.Context(), dto.ConfirmEmailInput{Token: token}))
}

func Test_ConfirmEmail_OpeningTheLinkAgainIsStillASuccess(t *testing.T) {
	d := newDeps(t)
	at := time.Now().Add(-time.Minute)
	c := pendingConfirmation(time.Now().Add(time.Hour))
	c.ConfirmedAt = &at

	d.expectLookup(c, domain.Profile{PartyID: alice, Email: "a@b.co", ConfirmedAt: &at})

	require.NoError(t, d.useCase(t, testConfig).ConfirmEmail(t.Context(), dto.ConfirmEmailInput{Token: token}))
}

func Test_ConfirmEmail_RejectsAnExpiredLink(t *testing.T) {
	d := newDeps(t)
	c := pendingConfirmation(time.Now().Add(-time.Second))

	d.expectLookup(c, domain.Profile{PartyID: alice, Email: "a@b.co"})

	err := d.useCase(t, testConfig).ConfirmEmail(t.Context(), dto.ConfirmEmailInput{Token: token})

	require.ErrorIs(t, err, domain.ErrConfirmationInvalid)
}

func Test_ConfirmEmail_RejectsAnUnknownLink(t *testing.T) {
	d := newDeps(t)

	d.postgres.EXPECT().GetConfirmation(Any, domain.HashToken(token)).Return(domain.Confirmation{}, domain.ErrNotFound)

	err := d.useCase(t, testConfig).ConfirmEmail(t.Context(), dto.ConfirmEmailInput{Token: token})

	require.ErrorIs(t, err, domain.ErrConfirmationInvalid)
}

func Test_ConfirmEmail_RejectsALinkForAnAddressThatWasReplaced(t *testing.T) {
	d := newDeps(t)
	c := pendingConfirmation(time.Now().Add(time.Hour))

	d.expectLookup(c, domain.Profile{PartyID: alice, Email: "new@b.co"})

	err := d.useCase(t, testConfig).ConfirmEmail(t.Context(), dto.ConfirmEmailInput{Token: token})

	require.ErrorIs(t, err, domain.ErrConfirmationInvalid)
}

func Test_ConfirmEmail_RejectsAnEmptyToken(t *testing.T) {
	d := newDeps(t)

	err := d.useCase(t, testConfig).ConfirmEmail(t.Context(), dto.ConfirmEmailInput{})

	require.ErrorIs(t, err, domain.ErrConfirmationInvalid)
}
