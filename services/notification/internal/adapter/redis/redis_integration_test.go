//go:build integration

package redis_test

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	redislib "github.com/vbloher/canton-private-otc-desk/pkg/redis"
	"github.com/vbloher/canton-private-otc-desk/services/notification/internal/adapter/redis"
)

const (
	testDB   = 9
	totalKey = "notification:confirmation:total"
)

func connect(t *testing.T) *redislib.Client {
	t.Helper()

	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		addr = "localhost:6379"
	}

	client, err := redislib.New(t.Context(), redislib.Config{Addr: addr, DB: testDB})
	require.NoError(t, err)
	t.Cleanup(client.Close)

	require.NoError(t, client.Del(t.Context(), totalKey).Err())

	return client
}

func newRedis(t *testing.T) *redis.Redis {
	t.Helper()

	return redis.New(connect(t), redis.LimitsFrom(redis.Config{DailyLimit: 1000}))
}

func Test_LimitsFrom_AreTheDocumentedOnes(t *testing.T) {
	require.Equal(t, redis.Limits{
		PartyCooldown: time.Minute, PartyPerDay: 10, EmailPerDay: 5, TotalPerDay: 250,
	}, redis.LimitsFrom(redis.Config{DailyLimit: 250}))
}

func Test_AllowConfirmation_APartyWaitsBetweenRequests(t *testing.T) {
	r := newRedis(t)
	party := uuid.NewString()

	first, err := r.AllowConfirmation(t.Context(), party, uuid.NewString()+"@b.co")
	require.NoError(t, err)
	require.True(t, first)

	second, err := r.AllowConfirmation(t.Context(), party, uuid.NewString()+"@b.co")
	require.NoError(t, err)
	require.False(t, second)
}

func Test_AllowConfirmation_AnAddressGetsFiveLettersADayFromAnyWallet(t *testing.T) {
	r := newRedis(t)
	email := uuid.NewString() + "@b.co"

	for i := range 5 {
		allowed, err := r.AllowConfirmation(t.Context(), uuid.NewString(), email)
		require.NoError(t, err)
		require.True(t, allowed, "letter %d", i+1)
	}

	allowed, err := r.AllowConfirmation(t.Context(), uuid.NewString(), email)
	require.NoError(t, err)
	require.False(t, allowed)
}

func Test_AllowConfirmation_SpellingsOfOneInboxShareTheirLimit(t *testing.T) {
	r := newRedis(t)
	name := "v" + strings.ReplaceAll(uuid.NewString(), "-", "")

	for i := range 5 {
		address := fmt.Sprintf("%s.%s+%d@gmail.com", name[:4], name[4:], i)

		allowed, err := r.AllowConfirmation(t.Context(), uuid.NewString(), address)
		require.NoError(t, err)
		require.True(t, allowed, "letter %d", i+1)
	}

	allowed, err := r.AllowConfirmation(t.Context(), uuid.NewString(), name+"@gmail.com")
	require.NoError(t, err)
	require.False(t, allowed)
}

func Test_AllowConfirmation_AWalletHasADailyLimitWhateverTheAddress(t *testing.T) {
	r := redis.New(connect(t), redis.Limits{PartyCooldown: time.Millisecond, PartyPerDay: 2, EmailPerDay: 5, TotalPerDay: 100})
	party := uuid.NewString()

	for i := range 3 {
		allowed, err := r.AllowConfirmation(t.Context(), party, uuid.NewString()+"@b.co")
		require.NoError(t, err)
		require.Equal(t, i < 2, allowed, "letter %d", i+1)

		time.Sleep(5 * time.Millisecond)
	}
}

func Test_AllowConfirmation_TheDeskHasADailyTotal(t *testing.T) {
	r := redis.New(connect(t), redis.Limits{PartyCooldown: time.Minute, PartyPerDay: 10, EmailPerDay: 5, TotalPerDay: 2})

	for i := range 3 {
		allowed, err := r.AllowConfirmation(t.Context(), uuid.NewString(), uuid.NewString()+"@b.co")
		require.NoError(t, err)
		require.Equal(t, i < 2, allowed, "letter %d", i+1)
	}
}

func Test_AllowConfirmation_CountersExpire(t *testing.T) {
	client := connect(t)
	r := redis.New(client, redis.LimitsFrom(redis.Config{DailyLimit: 1000}))

	allowed, err := r.AllowConfirmation(t.Context(), uuid.NewString(), uuid.NewString()+"@b.co")
	require.NoError(t, err)
	require.True(t, allowed)

	ttl, err := client.TTL(t.Context(), totalKey).Result()
	require.NoError(t, err)
	require.Greater(t, ttl, 23*time.Hour)
	require.LessOrEqual(t, ttl, 24*time.Hour)
}
