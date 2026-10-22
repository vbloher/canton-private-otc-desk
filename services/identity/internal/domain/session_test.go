package domain_test

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/domain"
)

func Test_NewSessionToken(t *testing.T) {
	token, hash := domain.NewSessionToken()

	raw, err := base64.RawURLEncoding.DecodeString(token)
	require.NoError(t, err, "the token is base64url without padding")
	require.Len(t, raw, 32, "the token is 32 random bytes")

	sum := sha256.Sum256([]byte(token))
	require.Equal(t, hex.EncodeToString(sum[:]), hash, "the hash is the hex sha256 of the token")
	require.Equal(t, hash, domain.HashSessionToken(token))

	other, otherHash := domain.NewSessionToken()
	require.NotEqual(t, token, other)
	require.NotEqual(t, hash, otherHash)
}

func Test_HashSessionToken_IsStable(t *testing.T) {
	require.Equal(t, domain.HashSessionToken("abc"), domain.HashSessionToken("abc"))
	require.NotEqual(t, domain.HashSessionToken("abc"), domain.HashSessionToken("abd"))
}
