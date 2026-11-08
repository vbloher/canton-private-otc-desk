package usecase_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/services/notification/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/notification/internal/usecase"
)

var (
	errBusy     = errors.New("resend is busy")
	errDatabase = errors.New("database is gone")
)

func due(key string) domain.Email {
	return domain.NewEmail(key, key+"@b.co", domain.KindTradeSettled, domain.Letter{Subject: "s", HTML: "h", Text: "t"}, time.Now())
}

func (d deps) expectClean() {
	d.postgres.EXPECT().Cleanup(Any, Any, Any).Return(nil).Once()
}

func (d deps) noEmails() {
	d.postgres.EXPECT().NextDueEmail(inTx(), Any).Return(domain.Email{}, domain.ErrNotFound).Once()
}

func (d deps) noContacts() {
	d.postgres.EXPECT().NextDueContactSync(inTx(), Any).Return(domain.ContactSync{}, domain.ErrNotFound).Once()
}

func withStatus(id uuid.UUID, status domain.DeliveryStatus) any {
	return mock.MatchedBy(func(e domain.Email) bool { return e.ID == id && e.Status == status })
}

func Test_Tick_SendsADueLetterInItsTransaction(t *testing.T) {
	d := newDeps(t)
	now := time.Now()
	e := due("one")

	d.expectClean()
	d.postgres.EXPECT().NextDueEmail(inTx(), now).Return(e, nil).Once()
	d.sender.EXPECT().Send(inTx(), e).Return("re_1", nil)
	d.postgres.EXPECT().UpdateEmail(inTx(), mock.MatchedBy(func(got domain.Email) bool {
		return got.ID == e.ID && got.Status == domain.DeliverySent && got.ProviderID == "re_1" && got.Recipient == ""
	})).Return(nil)
	d.noEmails()
	d.noContacts()

	require.NoError(t, d.useCase(t, testConfig).Tick(t.Context(), now))
}

func Test_Tick_ATemporaryFailureReschedulesAndEndsTheRound(t *testing.T) {
	d := newDeps(t)
	now := time.Now()
	e := due("one")

	d.expectClean()
	d.postgres.EXPECT().NextDueEmail(inTx(), now).Return(e, nil).Once()
	d.sender.EXPECT().Send(inTx(), e).Return("", errBusy)
	d.postgres.EXPECT().UpdateEmail(inTx(), mock.MatchedBy(func(got domain.Email) bool {
		return got.ID == e.ID && got.Status == domain.DeliveryPending && got.Attempts == 1 &&
			got.NextAttemptAt.Equal(now.Add(30*time.Second)) && got.Recipient == e.Recipient
	})).Return(nil)
	d.noContacts()

	require.NoError(t, d.useCase(t, testConfig).Tick(t.Context(), now))
}

func Test_Tick_AnUndeliverableLetterFailsAndTheNextOneGoesOut(t *testing.T) {
	d := newDeps(t)
	now := time.Now()
	bad := due("bad")
	good := due("good")

	d.expectClean()
	d.postgres.EXPECT().NextDueEmail(inTx(), now).Return(bad, nil).Once()
	d.sender.EXPECT().Send(inTx(), bad).Return("", domain.ErrUndeliverable)
	d.postgres.EXPECT().UpdateEmail(inTx(), withStatus(bad.ID, domain.DeliveryFailed)).Return(nil)
	d.postgres.EXPECT().NextDueEmail(inTx(), now).Return(good, nil).Once()
	d.sender.EXPECT().Send(inTx(), good).Return("re_2", nil)
	d.postgres.EXPECT().UpdateEmail(inTx(), withStatus(good.ID, domain.DeliverySent)).Return(nil)
	d.noEmails()
	d.noContacts()

	require.NoError(t, d.useCase(t, testConfig).Tick(t.Context(), now))
}

func Test_Tick_StopsAfterABatchOfLetters(t *testing.T) {
	d := newDeps(t)
	now := time.Now()
	e := due("one")

	d.expectClean()
	d.postgres.EXPECT().NextDueEmail(inTx(), now).Return(e, nil).Times(50)
	d.sender.EXPECT().Send(inTx(), e).Return("re_1", nil).Times(50)
	d.postgres.EXPECT().UpdateEmail(inTx(), Any).Return(nil).Times(50)
	d.noContacts()

	require.NoError(t, d.useCase(t, testConfig).Tick(t.Context(), now))
}

func Test_Tick_ReportsADatabaseFailureAndStillSyncsContacts(t *testing.T) {
	d := newDeps(t)
	now := time.Now()

	d.expectClean()
	d.postgres.EXPECT().NextDueEmail(inTx(), now).Return(domain.Email{}, errDatabase).Once()
	d.noContacts()

	require.ErrorIs(t, d.useCase(t, testConfig).Tick(t.Context(), now), errDatabase)
}

func (d deps) oneContact(now time.Time, s domain.ContactSync) {
	d.postgres.EXPECT().NextDueContactSync(inTx(), now).Return(s, nil).Once()
}

func Test_Tick_SubscribesAnAddressThatWantsNews(t *testing.T) {
	d := newDeps(t)
	now := time.Now()

	d.expectClean()
	d.noEmails()
	d.oneContact(now, domain.ContactSync{Email: "a@b.co"})
	d.postgres.EXPECT().NewsWanted(inTx(), "a@b.co").Return(true, nil)
	d.contacts.EXPECT().Subscribe(inTx(), "a@b.co").Return(nil)
	d.postgres.EXPECT().DeleteContactSync(inTx(), "a@b.co").Return(nil)
	d.noContacts()

	require.NoError(t, d.useCase(t, testConfig).Tick(t.Context(), now))
}

func Test_Tick_UnsubscribesAnAddressNobodyWantsNewsFor(t *testing.T) {
	d := newDeps(t)
	now := time.Now()

	d.expectClean()
	d.noEmails()
	d.oneContact(now, domain.ContactSync{Email: "a@b.co"})
	d.postgres.EXPECT().NewsWanted(inTx(), "a@b.co").Return(false, nil)
	d.contacts.EXPECT().Unsubscribe(inTx(), "a@b.co").Return(nil)
	d.postgres.EXPECT().DeleteContactSync(inTx(), "a@b.co").Return(nil)
	d.noContacts()

	require.NoError(t, d.useCase(t, testConfig).Tick(t.Context(), now))
}

func Test_Tick_AFailedContactSyncIsRescheduled(t *testing.T) {
	d := newDeps(t)
	now := time.Now()

	d.expectClean()
	d.noEmails()
	d.oneContact(now, domain.ContactSync{Email: "a@b.co"})
	d.postgres.EXPECT().NewsWanted(inTx(), "a@b.co").Return(true, nil)
	d.contacts.EXPECT().Subscribe(inTx(), "a@b.co").Return(errBusy)
	d.postgres.EXPECT().RescheduleContactSync(inTx(), domain.ContactSync{
		Email: "a@b.co", Attempts: 1, NextAttemptAt: now.Add(30 * time.Second),
	}).Return(nil)

	require.NoError(t, d.useCase(t, testConfig).Tick(t.Context(), now))
}

func Test_Tick_AContactSyncThatKeepsFailingIsDropped(t *testing.T) {
	d := newDeps(t)
	now := time.Now()

	d.expectClean()
	d.noEmails()
	d.oneContact(now, domain.ContactSync{Email: "a@b.co", Attempts: domain.MaxAttempts - 1})
	d.postgres.EXPECT().NewsWanted(inTx(), "a@b.co").Return(true, nil)
	d.contacts.EXPECT().Subscribe(inTx(), "a@b.co").Return(errBusy)
	d.postgres.EXPECT().DeleteContactSync(inTx(), "a@b.co").Return(nil)
	d.noContacts()

	require.NoError(t, d.useCase(t, testConfig).Tick(t.Context(), now))
}

func Test_Tick_CleansUpOnceAnHour(t *testing.T) {
	d := newDeps(t)
	now := time.Now()
	u := d.useCase(t, testConfig)

	d.postgres.EXPECT().Cleanup(Any, now, now.Add(-30*24*time.Hour)).Return(nil).Once()
	d.postgres.EXPECT().NextDueEmail(inTx(), Any).Return(domain.Email{}, domain.ErrNotFound)
	d.postgres.EXPECT().NextDueContactSync(inTx(), Any).Return(domain.ContactSync{}, domain.ErrNotFound)

	require.NoError(t, u.Tick(t.Context(), now))
	require.NoError(t, u.Tick(t.Context(), now.Add(time.Minute)))
}

func Test_Tick_DoesNothingWhenEmailIsOff(t *testing.T) {
	d := newDeps(t)

	require.NoError(t, d.useCase(t, usecase.Config{}).Tick(t.Context(), time.Now()))
}
