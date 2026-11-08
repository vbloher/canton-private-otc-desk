package usecase_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/services/notification/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/notification/internal/dto"
)

var hook = dto.ResendWebhookInput{Payload: []byte("{}"), ID: "id", Timestamp: "1", Signature: "sig"}

func (d deps) webhookEvent(event domain.ContactEvent, err error) {
	d.contacts.EXPECT().ParseWebhook(hook.Payload, hook.ID, hook.Timestamp, hook.Signature).Return(event, err)
}

func Test_HandleResendWebhook_AnUnsubscribeClearsNewsAndTellsTheOpenTabs(t *testing.T) {
	d := newDeps(t)

	d.webhookEvent(domain.ContactEvent{Email: "a@b.co", Unsubscribed: true}, nil)
	d.postgres.EXPECT().ClearNews(Any, "a@b.co").Return([]string{alice, bob}, nil)
	d.events.EXPECT().Publish(Any, profileEvent(), []string{alice, bob}).Return(nil)

	require.NoError(t, d.useCase(t, testConfig).HandleResendWebhook(t.Context(), hook))
}

func Test_HandleResendWebhook_AnAddressNobodyHasPublishesNothing(t *testing.T) {
	d := newDeps(t)

	d.webhookEvent(domain.ContactEvent{Email: "a@b.co", Unsubscribed: true}, nil)
	d.postgres.EXPECT().ClearNews(Any, "a@b.co").Return(nil, nil)

	require.NoError(t, d.useCase(t, testConfig).HandleResendWebhook(t.Context(), hook))
}

func Test_HandleResendWebhook_ASubscribeChangesNothing(t *testing.T) {
	d := newDeps(t)

	d.webhookEvent(domain.ContactEvent{Email: "a@b.co"}, nil)

	require.NoError(t, d.useCase(t, testConfig).HandleResendWebhook(t.Context(), hook))
}

func Test_HandleResendWebhook_OtherEventsChangeNothing(t *testing.T) {
	d := newDeps(t)

	d.webhookEvent(domain.ContactEvent{}, nil)

	require.NoError(t, d.useCase(t, testConfig).HandleResendWebhook(t.Context(), hook))
}

func Test_HandleResendWebhook_RejectsABadSignature(t *testing.T) {
	d := newDeps(t)

	d.webhookEvent(domain.ContactEvent{}, domain.ErrUnauthorized)

	require.ErrorIs(t, d.useCase(t, testConfig).HandleResendWebhook(t.Context(), hook), domain.ErrUnauthorized)
}
