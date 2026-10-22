package domain_test

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/domain"
)

func Test_NewChallenge(t *testing.T) {
	challenge := domain.NewChallenge()

	require.True(t, strings.HasPrefix(challenge, "otc-desk:"))

	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(challenge, "otc-desk:"))
	require.NoError(t, err, "the body is base64url without padding")
	require.Len(t, raw, 24, "the body is 24 random bytes")

	require.NotEqual(t, challenge, domain.NewChallenge())
}
