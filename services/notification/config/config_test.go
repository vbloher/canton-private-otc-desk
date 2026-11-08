package config_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/services/notification/config"
)

func setRequired(t *testing.T) {
	t.Helper()

	t.Setenv("POSTGRES_USER", "otc")
	t.Setenv("POSTGRES_PASSWORD", "otc")
	t.Setenv("POSTGRES_PORT", "5432")
	t.Setenv("POSTGRES_HOST", "localhost")
	t.Setenv("POSTGRES_DB_NAME", "notification")
	t.Setenv("REDIS_ADDR", "localhost:6379")
	t.Setenv("KAFKA_BROKERS", "kafka:9092")
}

func setResend(t *testing.T) {
	t.Helper()

	t.Setenv("RESEND_API_KEY", "re_key")
	t.Setenv("EMAIL_FROM", "Desk <desk@example.com>")
	t.Setenv("PUBLIC_URL", "https://desk.example.com")
}

func Test_New_Defaults(t *testing.T) {
	t.Chdir(t.TempDir())
	setRequired(t)

	c, err := config.New()
	require.NoError(t, err)

	require.Equal(t, "8084", c.HTTP.Port)
	require.Equal(t, "notification", c.OTEL.ServiceName)
	require.Equal(t, []string{"kafka:9092"}, c.Kafka.Brokers)
	require.Equal(t, "notification", c.Consumer.Group)
	require.Equal(t, 24*time.Hour, c.UseCase.ConfirmationTTL)
	require.False(t, c.UseCase.IncludeUpdateID)
	require.False(t, c.UseCase.Enabled)
	require.Equal(t, time.Second, c.Worker.Interval)
	require.Equal(t, 1000, c.Limits.DailyLimit)
}

func Test_New_AnEmptyLimitFallsBackToTheDefault(t *testing.T) {
	t.Chdir(t.TempDir())
	setRequired(t)
	t.Setenv("EMAIL_CONFIRMATIONS_DAILY_LIMIT", "")

	c, err := config.New()
	require.NoError(t, err)

	require.Equal(t, 1000, c.Limits.DailyLimit)
}

func Test_New_WithResendEmailIsOn(t *testing.T) {
	t.Chdir(t.TempDir())
	setRequired(t)
	setResend(t)

	c, err := config.New()
	require.NoError(t, err)

	require.True(t, c.UseCase.Enabled)
	require.False(t, c.LogEmails)
}

func Test_New_TheLogTransportTurnsEmailOn(t *testing.T) {
	t.Chdir(t.TempDir())
	setRequired(t)
	t.Setenv("EMAIL_LOG", "true")
	t.Setenv("PUBLIC_URL", "http://localhost:3000")

	c, err := config.New()
	require.NoError(t, err)

	require.True(t, c.UseCase.Enabled)
	require.True(t, c.LogEmails)
}

func Test_New_Rejects(t *testing.T) {
	cases := map[string]struct {
		env  map[string]string
		want string
	}{
		"a key without a sender":   {map[string]string{"EMAIL_FROM": ""}, "EMAIL_FROM"},
		"a key without a URL":      {map[string]string{"PUBLIC_URL": ""}, "PUBLIC_URL"},
		"a URL that is no URL":     {map[string]string{"PUBLIC_URL": "desk.example.com"}, "PUBLIC_URL"},
		"a token that never lives": {map[string]string{"EMAIL_CONFIRMATION_TTL": "0s"}, "EMAIL_CONFIRMATION_TTL"},
		"a daily limit of nothing": {map[string]string{"EMAIL_CONFIRMATIONS_DAILY_LIMIT": "0"}, "EMAIL_CONFIRMATIONS_DAILY_LIMIT"},
		"no brokers":               {map[string]string{"KAFKA_BROKERS": ""}, "KAFKA_BROKERS"},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			setRequired(t)
			setResend(t)

			for key, value := range c.env {
				t.Setenv(key, value)
			}

			_, err := config.New()
			require.ErrorContains(t, err, c.want)
		})
	}
}

func Test_New_TheLogTransportNeedsAURLToo(t *testing.T) {
	t.Chdir(t.TempDir())
	setRequired(t)
	t.Setenv("EMAIL_LOG", "true")

	_, err := config.New()
	require.ErrorContains(t, err, "PUBLIC_URL")
}
