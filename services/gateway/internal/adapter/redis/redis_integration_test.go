//go:build integration

package redis_test

import (
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel"
	"github.com/vbloher/canton-private-otc-desk/pkg/presence"
	redislib "github.com/vbloher/canton-private-otc-desk/pkg/redis"
	"github.com/vbloher/canton-private-otc-desk/services/gateway/internal/adapter/redis"
)

func connect(t *testing.T) (*redis.Redis, *redislib.Client) {
	t.Helper()

	otel.SilentModeInit()

	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		addr = "localhost:6379"
	}

	client, err := redislib.New(t.Context(), redislib.Config{Addr: addr})
	require.NoError(t, err)
	t.Cleanup(client.Close)

	return redis.New(client), client
}

func Test_MarkOnline_And_ClearOnline(t *testing.T) {
	r, client := connect(t)

	alice := "alice-" + uuid.NewString()[:8] + "::1220"
	bob := "bob-" + uuid.NewString()[:8] + "::1220"

	require.NoError(t, r.MarkOnline(t.Context(), []string{alice, bob}, time.Minute))

	for _, party := range []string{alice, bob} {
		ttl, err := client.TTL(t.Context(), presence.Key(party)).Result()
		require.NoError(t, err)
		require.Greater(t, ttl, time.Duration(0), party)
		require.LessOrEqual(t, ttl, time.Minute, party)
	}

	require.NoError(t, r.ClearOnline(t.Context(), alice))

	left, err := client.Exists(t.Context(), presence.Key(alice), presence.Key(bob)).Result()
	require.NoError(t, err)
	require.Equal(t, int64(1), left, "only the cleared party goes offline")
}

func Test_MarkOnline_Nobody(t *testing.T) {
	r, _ := connect(t)

	require.NoError(t, r.MarkOnline(t.Context(), nil, time.Minute))
}
