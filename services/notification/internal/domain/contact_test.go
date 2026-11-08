package domain_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/services/notification/internal/domain"
)

func Test_ContactSync_RetryBacksOffAndKeepsTheAddress(t *testing.T) {
	now := time.Now()

	s := domain.ContactSync{Email: "a@b.co", Attempts: 2}.Retry(now)

	require.Equal(t, "a@b.co", s.Email)
	require.Equal(t, 3, s.Attempts)
	require.True(t, s.NextAttemptAt.Equal(now.Add(2*time.Minute)))
}

func Test_ContactSync_GivesUpAfterTheLastAttempt(t *testing.T) {
	require.False(t, domain.ContactSync{Attempts: domain.MaxAttempts - 2}.Retry(time.Now()).Exhausted())
	require.True(t, domain.ContactSync{Attempts: domain.MaxAttempts - 1}.Retry(time.Now()).Exhausted())
}
