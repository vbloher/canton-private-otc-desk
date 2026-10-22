package usecase_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/dto"
)

func Test_Authenticate_CacheHit(t *testing.T) {
	d := newDeps(t) // no Postgres expectation: a cache hit never reaches the database

	user, _ := walletUser(t)
	hash := domain.HashSessionToken("token")

	d.redis.EXPECT().GetCachedSession(Any, hash).Return(user, true)

	output, err := d.useCase().Authenticate(t.Context(), dto.AuthenticateInput{Token: "token"})
	require.NoError(t, err)
	require.Equal(t, dto.AuthenticateOutput{User: user}, output)
}

func Test_Authenticate_Miss_CachesUser(t *testing.T) {
	d := newDeps(t)

	user, _ := walletUser(t)
	hash := domain.HashSessionToken("token")

	d.redis.EXPECT().GetCachedSession(Any, hash).Return(domain.User{}, false)
	d.postgres.EXPECT().GetUserBySession(Any, hash, mock.MatchedBy(func(now time.Time) bool {
		return time.Since(now) < time.Minute
	})).Return(user, nil)
	d.redis.EXPECT().CacheSession(Any, hash, user, time.Minute).Return()

	output, err := d.useCase().Authenticate(t.Context(), dto.AuthenticateInput{Token: "token"})
	require.NoError(t, err)
	require.Equal(t, dto.AuthenticateOutput{User: user}, output)
}

func Test_Authenticate_UnknownOrExpiredSession(t *testing.T) {
	d := newDeps(t)

	hash := domain.HashSessionToken("token")

	d.redis.EXPECT().GetCachedSession(Any, hash).Return(domain.User{}, false)
	d.postgres.EXPECT().GetUserBySession(Any, hash, Any).Return(domain.User{}, domain.ErrNotFound)
	// No CacheSession expectation: a miss is not cached.

	_, err := d.useCase().Authenticate(t.Context(), dto.AuthenticateInput{Token: "token"})
	require.ErrorIs(t, err, domain.ErrUnauthorized)
}

func Test_Authenticate_EmptyToken(t *testing.T) {
	d := newDeps(t) // no expectations

	_, err := d.useCase().Authenticate(t.Context(), dto.AuthenticateInput{Token: ""})
	require.ErrorIs(t, err, domain.ErrUnauthorized)
}
