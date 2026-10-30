//go:build integration

package redis_test

import (
	"bytes"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel"
	redislib "github.com/vbloher/canton-private-otc-desk/pkg/redis"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/adapter/redis"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/domain"
)

func newRedis(t *testing.T) *redis.Redis {
	t.Helper()

	otel.SilentModeInit()

	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		addr = "localhost:6379"
	}

	client, err := redislib.New(t.Context(), redislib.Config{Addr: addr})
	require.NoError(t, err)
	t.Cleanup(client.Close)

	return redis.New(client)
}

func registration(t *testing.T) domain.Registration {
	t.Helper()

	reg, err := domain.NewRegistration("Alice Smith", bytes.Repeat([]byte{3}, 32))
	require.NoError(t, err)

	reg.Topology = domain.Topology{
		PartyID:      reg.Hint + "::1220ab",
		Fingerprint:  "1220ab",
		MultiHash:    []byte{0, 1, 2, 255},
		Transactions: []string{"tx-1", "tx-2"},
	}

	return reg
}

func Test_Registration_IsTakenOnce(t *testing.T) {
	r := newRedis(t)

	reg := registration(t)
	require.NoError(t, r.SaveRegistration(t.Context(), reg, time.Minute))

	got, err := r.TakeRegistration(t.Context(), reg.ID)
	require.NoError(t, err)
	require.Equal(t, reg, got, "the whole topology survives the round trip")

	_, err = r.TakeRegistration(t.Context(), reg.ID)
	require.ErrorIs(t, err, domain.ErrRegistrationExpired, "a registration completes once")
}

func Test_Registration_GetKeepsItForTheTake(t *testing.T) {
	r := newRedis(t)

	reg := registration(t)
	reg.PartyID = reg.Topology.PartyID
	require.NoError(t, r.SaveRegistration(t.Context(), reg, time.Minute))

	got, err := r.GetRegistration(t.Context(), reg.ID)
	require.NoError(t, err)
	require.Equal(t, reg, got, "the created party survives the round trip")

	got, err = r.TakeRegistration(t.Context(), reg.ID)
	require.NoError(t, err)
	require.Equal(t, reg, got)

	_, err = r.GetRegistration(t.Context(), reg.ID)
	require.ErrorIs(t, err, domain.ErrRegistrationExpired)
}

func Test_Registration_Unknown(t *testing.T) {
	r := newRedis(t)

	_, err := r.TakeRegistration(t.Context(), uuid.New())
	require.ErrorIs(t, err, domain.ErrRegistrationExpired)
}

func Test_Registration_Expires(t *testing.T) {
	r := newRedis(t)

	reg := registration(t)
	require.NoError(t, r.SaveRegistration(t.Context(), reg, 50*time.Millisecond))

	time.Sleep(150 * time.Millisecond)

	_, err := r.TakeRegistration(t.Context(), reg.ID)
	require.ErrorIs(t, err, domain.ErrRegistrationExpired)
}

func Test_Registration_OnlyOneConcurrentTakeWins(t *testing.T) {
	r := newRedis(t)

	reg := registration(t)
	require.NoError(t, r.SaveRegistration(t.Context(), reg, time.Minute))

	var (
		wins atomic.Int32
		wg   sync.WaitGroup
	)

	for range 20 {
		wg.Add(1)

		go func() {
			defer wg.Done()

			_, err := r.TakeRegistration(t.Context(), reg.ID)
			if err == nil {
				wins.Add(1)
			}
		}()
	}

	wg.Wait()

	require.EqualValues(t, 1, wins.Load())
}

func Test_Challenge_IsTakenOnce(t *testing.T) {
	r := newRedis(t)

	userID := uuid.New()
	challenge := domain.NewChallenge()

	require.NoError(t, r.SaveChallenge(t.Context(), challenge, userID, time.Minute))

	got, err := r.TakeChallenge(t.Context(), challenge)
	require.NoError(t, err)
	require.Equal(t, userID, got)

	_, err = r.TakeChallenge(t.Context(), challenge)
	require.ErrorIs(t, err, domain.ErrUnauthorized, "a challenge works once")
}

func Test_Challenge_UnknownAndExpired(t *testing.T) {
	r := newRedis(t)

	_, err := r.TakeChallenge(t.Context(), domain.NewChallenge())
	require.ErrorIs(t, err, domain.ErrUnauthorized)

	challenge := domain.NewChallenge()
	require.NoError(t, r.SaveChallenge(t.Context(), challenge, uuid.New(), 50*time.Millisecond))

	time.Sleep(150 * time.Millisecond)

	_, err = r.TakeChallenge(t.Context(), challenge)
	require.ErrorIs(t, err, domain.ErrUnauthorized)
}

func Test_SessionCache(t *testing.T) {
	r := newRedis(t)

	user, err := domain.NewPasskeyUser(
		"alice-3f9a1c::1220ab", "alice-3f9a1c", "Alice Smith", bytes.Repeat([]byte{7}, 32), "cred-1", "blob-1",
	)
	require.NoError(t, err)

	user.APIKeyHash = "some-hash"
	user.MakerMode = true

	hash := domain.HashSessionToken(uuid.NewString())

	_, ok := r.GetCachedSession(t.Context(), hash)
	require.False(t, ok, "nothing cached yet")

	r.CacheSession(t.Context(), hash, user, time.Minute)

	got, ok := r.GetCachedSession(t.Context(), hash)
	require.True(t, ok)
	require.WithinDuration(t, user.CreatedAt, got.CreatedAt, time.Millisecond)

	want := user
	want.CreatedAt, got.CreatedAt = time.Time{}, time.Time{}
	want.EncryptedBlob, want.APIKeyHash = "", ""
	require.Equal(t, want, got, "everything but the wrapped key and the API key hash is cached")

	r.DropCachedSession(t.Context(), hash)

	_, ok = r.GetCachedSession(t.Context(), hash)
	require.False(t, ok, "dropped on logout")

	r.DropCachedSession(t.Context(), hash)
}

func Test_SessionCache_Expires(t *testing.T) {
	r := newRedis(t)

	hash := domain.HashSessionToken(uuid.NewString())
	r.CacheSession(t.Context(), hash, domain.User{ID: uuid.New()}, 50*time.Millisecond)

	time.Sleep(150 * time.Millisecond)

	_, ok := r.GetCachedSession(t.Context(), hash)
	require.False(t, ok)
}
