package domain_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/services/notification/internal/domain"
)

func pendingEmail(now time.Time) domain.Email {
	return domain.NewEmail("key", "a@b.co", domain.KindTradeSettled, domain.Letter{
		Subject: "subject", HTML: "<p>body</p>", Text: "body",
	}, now)
}

func Test_NewEmail_IsDueAtOnce(t *testing.T) {
	now := time.Now()

	e := pendingEmail(now)

	require.Equal(t, domain.DeliveryPending, e.Status)
	require.True(t, e.NextAttemptAt.Equal(now))
	require.NotEmpty(t, e.ID)
}

func Test_Email_DeliveredKeepsNoAddressOrBody(t *testing.T) {
	now := time.Now()

	e := pendingEmail(now).Delivered("re_1", now)

	require.Equal(t, domain.DeliverySent, e.Status)
	require.Equal(t, "re_1", e.ProviderID)
	require.True(t, e.FinishedAt.Equal(now))
	require.Empty(t, e.Recipient)
	require.Empty(t, e.Subject)
	require.Empty(t, e.HTML)
	require.Empty(t, e.Text)
}

func Test_Email_UndeliverableFailsAtOnceAndKeepsNoAddressOrBody(t *testing.T) {
	now := time.Now()

	e := pendingEmail(now).Undeliverable("rejected", now)

	require.Equal(t, domain.DeliveryFailed, e.Status)
	require.Equal(t, "rejected", e.LastError)
	require.Empty(t, e.Recipient)
	require.Empty(t, e.HTML)
}

func Test_Email_RetryBacksOffDoublingUpToAnHour(t *testing.T) {
	now := time.Now()
	e := pendingEmail(now)

	for _, want := range []time.Duration{
		30 * time.Second, time.Minute, 2 * time.Minute, 4 * time.Minute, 8 * time.Minute,
		16 * time.Minute, 32 * time.Minute, time.Hour, time.Hour,
	} {
		e = e.Retry("busy", now)

		require.Equal(t, domain.DeliveryPending, e.Status)
		require.True(t, e.NextAttemptAt.Equal(now.Add(want)), "after %d attempts, want a delay of %s", e.Attempts, want)
		require.Equal(t, "a@b.co", e.Recipient, "a retry needs the address")
	}
}

func Test_Email_TheTenthFailureIsFinal(t *testing.T) {
	now := time.Now()
	e := pendingEmail(now)

	for range 9 {
		e = e.Retry("busy", now)
	}

	require.Equal(t, domain.DeliveryPending, e.Status)

	e = e.Retry("busy", now)

	require.Equal(t, domain.DeliveryFailed, e.Status)
	require.Equal(t, 10, e.Attempts)
	require.Empty(t, e.Recipient)
}

func Test_Email_KeepsOnlyTheStartOfALongError(t *testing.T) {
	e := pendingEmail(time.Now()).Retry(strings.Repeat("x", 5000), time.Now())

	require.Len(t, e.LastError, 500)
}
