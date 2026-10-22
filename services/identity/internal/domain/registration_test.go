package domain_test

import (
	"crypto/ed25519"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/domain"
)

func Test_NewRegistration(t *testing.T) {
	reg, err := domain.NewRegistration("  Alice Smith ", key32())
	require.NoError(t, err)

	require.NotEqual(t, uuid.Nil, reg.ID)
	require.Equal(t, "Alice Smith", reg.DisplayName)
	require.Equal(t, key32(), reg.PublicKey)
	require.Regexp(t, `^alice-smith-[0-9a-f]{6}$`, reg.Hint)
	require.Equal(t, domain.Topology{}, reg.Topology, "the topology is filled in by the ledger")
}

func Test_NewRegistration_Validation(t *testing.T) {
	for name, c := range map[string]struct {
		displayName string
		publicKey   []byte
	}{
		"short name": {"A", key32()},
		"long name":  {strings.Repeat("x", 33), key32()},
		"short key":  {"Alice", key32()[:31]},
		"no key":     {"Alice", nil},
	} {
		_, err := domain.NewRegistration(c.displayName, c.publicKey)
		require.ErrorIs(t, err, domain.ErrValidation, name)
	}
}

func Test_Registration_User(t *testing.T) {
	reg, err := domain.NewRegistration("Alice", key32())
	require.NoError(t, err)

	user, err := reg.NewUser("alice::1220ab", "cred-1", "blob-1")
	require.NoError(t, err)

	require.Equal(t, reg.Hint, user.Hint)
	require.Equal(t, "Alice", user.DisplayName)
	require.Equal(t, key32(), user.PublicKey)
	require.Equal(t, "alice::1220ab", user.PartyID)
	require.Equal(t, "cred-1", user.CredentialID)
	require.Equal(t, domain.KindPasskey, user.Kind)

	_, err = reg.NewUser("alice::1220ab", "", "blob-1")
	require.ErrorIs(t, err, domain.ErrValidation)
}

func Test_Registration_VerifySignature(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	reg, err := domain.NewRegistration("Alice", publicKey)
	require.NoError(t, err)

	reg.Topology.MultiHash = []byte("the onboarding hash")

	require.NoError(t, reg.VerifySignature(ed25519.Sign(privateKey, reg.Topology.MultiHash)))

	err = reg.VerifySignature(ed25519.Sign(privateKey, []byte("some other hash")))
	require.ErrorIs(t, err, domain.ErrValidation)
	require.ErrorContains(t, err, "onboarding hash")
}
