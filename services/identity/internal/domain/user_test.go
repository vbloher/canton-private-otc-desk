package domain_test

import (
	"bytes"
	"regexp"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/domain"
)

func key32() []byte {
	return bytes.Repeat([]byte{7}, 32)
}

func Test_NewPasskeyUser_Validation(t *testing.T) {
	cases := []struct {
		name        string
		displayName string
		publicKey   []byte
		credential  string
		blob        string
	}{
		{name: "one rune name", displayName: "A", publicKey: key32(), credential: "cred", blob: "blob"},
		{name: "blank name", displayName: "   ", publicKey: key32(), credential: "cred", blob: "blob"},
		{name: "name too short once trimmed", displayName: " A ", publicKey: key32(), credential: "cred", blob: "blob"},
		{name: "one multi byte rune", displayName: "Ж", publicKey: key32(), credential: "cred", blob: "blob"},
		{name: "33 runes", displayName: strings.Repeat("Ж", 33), publicKey: key32(), credential: "cred", blob: "blob"},
		{name: "control character", displayName: "Al\nice", publicKey: key32(), credential: "cred", blob: "blob"},
		{name: "31 byte key", displayName: "Alice", publicKey: key32()[:31], credential: "cred", blob: "blob"},
		{name: "33 byte key", displayName: "Alice", publicKey: append(key32(), 1), credential: "cred", blob: "blob"},
		{name: "no key", displayName: "Alice", publicKey: nil, credential: "cred", blob: "blob"},
		{name: "no credential", displayName: "Alice", publicKey: key32(), credential: "", blob: "blob"},
		{name: "no blob", displayName: "Alice", publicKey: key32(), credential: "cred", blob: ""},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := domain.NewPasskeyUser("alice-3f9a1c::1220ab", "alice-3f9a1c", c.displayName, c.publicKey, c.credential, c.blob)
			require.ErrorIs(t, err, domain.ErrValidation)
			require.True(t, domain.IsInvalidInput(err))
		})
	}
}

func Test_NewPasskeyUser_Validation_NamesTheProblem(t *testing.T) {
	_, err := domain.NewPasskeyUser("alice::1220ab", "alice-3f9a1c", "A", key32(), "cred", "blob")
	require.ErrorContains(t, err, "display name")

	_, err = domain.NewPasskeyUser("alice::1220ab", "alice-3f9a1c", "Alice", key32()[:31], "cred", "blob")
	require.ErrorContains(t, err, "public key")
}

func Test_NewPasskeyUser_Success(t *testing.T) {
	user, err := domain.NewPasskeyUser("alice-3f9a1c::1220ab", "alice-3f9a1c", "  Alice  ", key32(), "cred-1", "blob-1")
	require.NoError(t, err)

	require.NotEqual(t, uuid.Nil, user.ID)
	require.Equal(t, "alice-3f9a1c::1220ab", user.PartyID)
	require.Equal(t, "alice-3f9a1c", user.Hint)
	require.Equal(t, "Alice", user.DisplayName, "the name is stored trimmed")
	require.Equal(t, domain.KindPasskey, user.Kind)
	require.Equal(t, key32(), user.PublicKey)
	require.Equal(t, "cred-1", user.CredentialID)
	require.Equal(t, "blob-1", user.EncryptedBlob)
	require.Empty(t, user.APIKeyHash)
	require.False(t, user.MakerMode, "a wallet is not a maker until it opts in")
	require.False(t, user.CreatedAt.IsZero())
}

func Test_NewPasskeyUser_AcceptsBoundaryNames(t *testing.T) {
	for _, name := range []string{"Al", strings.Repeat("Ж", 32)} {
		_, err := domain.NewPasskeyUser("p::1", "p-aaaaaa", name, key32(), "cred", "blob")
		require.NoError(t, err, name)
	}
}

func Test_NewHouseUser(t *testing.T) {
	user, err := domain.NewHouseUser("makerA::1220ab", "makerA", "demo-key-a")
	require.NoError(t, err)

	require.NotEqual(t, uuid.Nil, user.ID)
	require.Equal(t, "makerA::1220ab", user.PartyID)
	require.Equal(t, "makerA", user.Hint)
	require.Equal(t, "makerA", user.DisplayName)
	require.Equal(t, domain.KindHouse, user.Kind)
	require.True(t, user.MakerMode)
	require.Equal(t, domain.HashAPIKey("demo-key-a"), user.APIKeyHash)
	require.Empty(t, user.CredentialID)
	require.Empty(t, user.EncryptedBlob)
	require.Nil(t, user.PublicKey)
}

func Test_NewHouseUser_Validation(t *testing.T) {
	for _, c := range []struct{ partyID, hint, apiKey string }{
		{"", "makerA", "key"},
		{"makerA::1220ab", "", "key"},
		{"makerA::1220ab", "makerA", ""},
	} {
		_, err := domain.NewHouseUser(c.partyID, c.hint, c.apiKey)
		require.ErrorIs(t, err, domain.ErrValidation, "%+v", c)
	}
}

func Test_HashAPIKey(t *testing.T) {
	hash := domain.HashAPIKey("demo-key-a")

	require.Regexp(t, regexp.MustCompile(`^[0-9a-f]{64}$`), hash)
	require.Equal(t, hash, domain.HashAPIKey("demo-key-a"))
	require.NotEqual(t, hash, domain.HashAPIKey("demo-key-b"))
	require.Equal(t, "f15424e984f66fb993b9d8e6444ade02adcc714a58b3b50106e98750e4244bfe", hash)
}

func Test_ValidateCredential(t *testing.T) {
	require.NoError(t, domain.ValidateCredential("cred-1", "blob-1"))

	err := domain.ValidateCredential("", "blob-1")
	require.ErrorIs(t, err, domain.ErrValidation)
	require.ErrorContains(t, err, "credential id")

	err = domain.ValidateCredential("cred-1", "")
	require.ErrorIs(t, err, domain.ErrValidation)
	require.ErrorContains(t, err, "encrypted blob")

	require.ErrorIs(t, domain.ValidateCredential("", ""), domain.ErrValidation)
}
