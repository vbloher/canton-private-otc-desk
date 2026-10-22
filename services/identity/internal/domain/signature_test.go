package domain_test

import (
	"crypto/ed25519"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/domain"
)

func Test_VerifySignature(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	message := []byte("otc-desk:4kJ8m2")
	signature := ed25519.Sign(privateKey, message)

	t.Run("valid", func(t *testing.T) {
		require.True(t, domain.VerifySignature(publicKey, message, signature))
	})

	t.Run("tampered message", func(t *testing.T) {
		require.False(t, domain.VerifySignature(publicKey, []byte("otc-desk:4kJ8m3"), signature))
	})

	t.Run("tampered signature", func(t *testing.T) {
		bad := append([]byte(nil), signature...)
		bad[0] ^= 0xff

		require.False(t, domain.VerifySignature(publicKey, message, bad))
	})

	t.Run("another key", func(t *testing.T) {
		other, _, err := ed25519.GenerateKey(nil)
		require.NoError(t, err)

		require.False(t, domain.VerifySignature(other, message, signature))
	})

	t.Run("wrong lengths never panic", func(t *testing.T) {
		require.False(t, domain.VerifySignature(publicKey[:31], message, signature))
		require.False(t, domain.VerifySignature(publicKey, message, signature[:63]))
		require.False(t, domain.VerifySignature(nil, message, nil))
	})
}

func Test_ValidateSignature(t *testing.T) {
	require.NoError(t, domain.ValidateSignature(make([]byte, 64)))

	for _, size := range []int{0, 1, 63, 65, 128} {
		err := domain.ValidateSignature(make([]byte, size))
		require.ErrorIs(t, err, domain.ErrValidation, "size %d", size)
		require.ErrorContains(t, err, "signature")
	}
}
