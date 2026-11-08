package domain_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/services/notification/internal/domain"
)

func Test_NewConfirmation_StoresOnlyTheHashOfTheToken(t *testing.T) {
	now := time.Now()

	c, token := domain.NewConfirmation("alice::1220", "a@b.co", now, 24*time.Hour)

	require.GreaterOrEqual(t, len(token), 43)
	require.NotEqual(t, token, c.TokenHash)
	require.Equal(t, domain.HashToken(token), c.TokenHash)
	require.False(t, strings.ContainsAny(token, "+/="), "the token goes into a URL")
	require.Equal(t, "alice::1220", c.PartyID)
	require.Equal(t, "a@b.co", c.Email)
	require.True(t, c.ExpiresAt.Equal(now.Add(24*time.Hour)))
}

func Test_NewConfirmation_NeverRepeatsAToken(t *testing.T) {
	_, first := domain.NewConfirmation("p", "a@b.co", time.Now(), time.Hour)
	_, second := domain.NewConfirmation("p", "a@b.co", time.Now(), time.Hour)

	require.NotEqual(t, first, second)
}

func Test_Confirmation_ExpiresAtItsDeadline(t *testing.T) {
	now := time.Now()
	c := domain.Confirmation{ExpiresAt: now}

	require.False(t, c.Expired(now.Add(-time.Second)))
	require.True(t, c.Expired(now))
}
