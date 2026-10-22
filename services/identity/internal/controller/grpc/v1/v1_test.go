package v1_test

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/vbloher/canton-private-otc-desk/gen/grpc/identity_v1"
	"github.com/vbloher/canton-private-otc-desk/pkg/otel"
	v1 "github.com/vbloher/canton-private-otc-desk/services/identity/internal/controller/grpc/v1"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/usecase"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/usecase/mocks"
)

const Any = mock.Anything

type fixture struct {
	handlers *v1.Handlers
	postgres *mocks.Postgres
	redis    *mocks.Redis
}

func newFixture(t *testing.T) fixture {
	t.Helper()

	otel.SilentModeInit()

	f := fixture{postgres: mocks.NewPostgres(t), redis: mocks.NewRedis(t)}

	f.handlers = v1.New(usecase.New(f.postgres, f.redis, mocks.NewLedger(t), usecase.Config{SessionTTL: time.Hour}))

	return f
}

func alice(t *testing.T) domain.User {
	t.Helper()

	user, err := domain.NewPasskeyUser(
		"alice-3f9a1c::1220ab", "alice-3f9a1c", "Alice", bytes.Repeat([]byte{7}, 32), "cred-1", "blob-1",
	)
	require.NoError(t, err)

	user.MakerMode = true

	return user
}

func requireCode(t *testing.T, want codes.Code, err error) {
	t.Helper()

	require.Error(t, err)
	require.Equal(t, want, status.Code(err), err.Error())
}

func Test_Authenticate_Session(t *testing.T) {
	f := newFixture(t)

	user := alice(t)
	hash := domain.HashSessionToken("token")

	f.redis.EXPECT().GetCachedSession(Any, hash).Return(domain.User{}, false)
	f.postgres.EXPECT().GetUserBySession(Any, hash, Any).Return(user, nil)
	f.redis.EXPECT().CacheSession(Any, hash, user, time.Minute).Return()

	got, err := f.handlers.Authenticate(t.Context(), &pb.AuthenticateInput{SessionToken: "token"})
	require.NoError(t, err)

	require.Equal(t, user.ID.String(), got.GetId())
	require.Equal(t, "alice-3f9a1c::1220ab", got.GetPartyId())
	require.Equal(t, "alice-3f9a1c", got.GetHint())
	require.Equal(t, "Alice", got.GetDisplayName())
	require.True(t, got.GetMakerMode())
}

func Test_Authenticate_UnknownOrExpiredSessionIsUnauthenticated(t *testing.T) {
	f := newFixture(t)

	hash := domain.HashSessionToken("stale")

	f.redis.EXPECT().GetCachedSession(Any, hash).Return(domain.User{}, false)
	f.postgres.EXPECT().GetUserBySession(Any, hash, Any).Return(domain.User{}, domain.ErrNotFound)

	_, err := f.handlers.Authenticate(t.Context(), &pb.AuthenticateInput{SessionToken: "stale"})
	requireCode(t, codes.Unauthenticated, err)
}

func Test_Authenticate_NoTokenIsUnauthenticated(t *testing.T) {
	f := newFixture(t) // no expectations

	_, err := f.handlers.Authenticate(t.Context(), &pb.AuthenticateInput{})
	requireCode(t, codes.Unauthenticated, err)
}

func Test_Authenticate_StorageFailureIsInternalAndQuiet(t *testing.T) {
	f := newFixture(t)

	f.redis.EXPECT().GetCachedSession(Any, Any).Return(domain.User{}, false)
	f.postgres.EXPECT().GetUserBySession(Any, Any, Any).
		Return(domain.User{}, errors.New("dial tcp 10.0.0.5:5432: connection refused"))

	_, err := f.handlers.Authenticate(t.Context(), &pb.AuthenticateInput{SessionToken: "token"})
	requireCode(t, codes.Internal, err)
	require.NotContains(t, err.Error(), "10.0.0.5")
}

func Test_AuthenticateAPIKey(t *testing.T) {
	f := newFixture(t)

	maker, err := domain.NewHouseUser("makerA::1220ab", "makerA", "demo-key-a")
	require.NoError(t, err)

	f.postgres.EXPECT().GetUserByAPIKeyHash(Any, domain.HashAPIKey("demo-key-a")).Return(maker, nil)

	got, err := f.handlers.AuthenticateAPIKey(t.Context(), &pb.AuthenticateAPIKeyInput{ApiKey: "demo-key-a"})
	require.NoError(t, err)

	require.Equal(t, maker.ID.String(), got.GetId())
	require.Equal(t, "makerA::1220ab", got.GetPartyId())
	require.Equal(t, "makerA", got.GetHint())
	require.Equal(t, "makerA", got.GetDisplayName())
	require.True(t, got.GetMakerMode())
}

func Test_AuthenticateAPIKey_UnknownKeyIsUnauthenticated(t *testing.T) {
	f := newFixture(t)

	f.postgres.EXPECT().GetUserByAPIKeyHash(Any, Any).Return(domain.User{}, domain.ErrNotFound)

	_, err := f.handlers.AuthenticateAPIKey(t.Context(), &pb.AuthenticateAPIKeyInput{ApiKey: "guess"})
	requireCode(t, codes.Unauthenticated, err)
}

func Test_AuthenticateAPIKey_NoKeyIsUnauthenticated(t *testing.T) {
	f := newFixture(t) // no expectations

	_, err := f.handlers.AuthenticateAPIKey(t.Context(), &pb.AuthenticateAPIKeyInput{})
	requireCode(t, codes.Unauthenticated, err)
}

func Test_ResolveMakers(t *testing.T) {
	f := newFixture(t)

	makerA := domain.User{ID: uuid.New(), PartyID: "makerA::1", Hint: "makerA", DisplayName: "makerA", MakerMode: true}
	makerB := domain.User{ID: uuid.New(), PartyID: "makerB::1", Hint: "makerB", DisplayName: "makerB", MakerMode: true}

	f.postgres.EXPECT().ListMakers(Any).Return([]domain.User{makerA, makerB}, nil)

	got, err := f.handlers.ResolveMakers(t.Context(), &pb.ResolveMakersInput{Hints: []string{"makerB", "ghost"}})
	require.NoError(t, err)

	require.Len(t, got.GetMakers(), 1, "unknown hints are left out")
	require.Equal(t, makerB.ID.String(), got.GetMakers()[0].GetId())
	require.Equal(t, "makerB::1", got.GetMakers()[0].GetPartyId())
	require.Equal(t, "makerB", got.GetMakers()[0].GetHint())
	require.True(t, got.GetMakers()[0].GetMakerMode())
}

func Test_ResolveMakers_NoneFoundIsAnEmptyAnswer(t *testing.T) {
	f := newFixture(t)

	f.postgres.EXPECT().ListMakers(Any).Return(nil, nil)

	got, err := f.handlers.ResolveMakers(t.Context(), &pb.ResolveMakersInput{Hints: []string{"ghost"}})
	require.NoError(t, err)
	require.Empty(t, got.GetMakers())
}

func Test_ResolveMakers_StorageFailureIsInternal(t *testing.T) {
	f := newFixture(t)

	f.postgres.EXPECT().ListMakers(Any).Return(nil, errors.New("boom"))

	_, err := f.handlers.ResolveMakers(t.Context(), &pb.ResolveMakersInput{Hints: []string{"makerA"}})
	requireCode(t, codes.Internal, err)
}
