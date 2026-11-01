package identity_test

import (
	"crypto/ed25519"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/maker-bot/internal/identity"
)

func TestSaveLoad_RoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "identity.json")

	id, err := identity.Generate()
	require.NoError(t, err)

	id.PartyID = "otc-demo-maker-1a2b3c::1220aa"
	id.Hint = "otc-demo-maker-1a2b3c"
	id.RegistrationID = uuid.New()
	id.MultiHash = []byte{1, 2, 3}
	id.Complete = true

	require.NoError(t, identity.Save(path, id))

	got, err := identity.Load(path)
	require.NoError(t, err)
	require.Equal(t, id.PartyID, got.PartyID)
	require.Equal(t, id.Hint, got.Hint)
	require.Equal(t, id.RegistrationID, got.RegistrationID)
	require.Equal(t, id.MultiHash, got.MultiHash)
	require.True(t, got.Complete)
	require.Equal(t, id.PublicKey(), got.PublicKey())
	require.True(t, ed25519.Verify(id.PublicKey(), []byte("hello"), got.Sign([]byte("hello"))),
		"the loaded key signs for the saved public key")
}

func TestSave_Mode0600(t *testing.T) {
	path := filepath.Join(t.TempDir(), "identity.json")

	id, err := identity.Generate()
	require.NoError(t, err)
	require.NoError(t, identity.Save(path, id))

	info, err := os.Stat(path)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func TestSave_ReplacesTheFileWhole(t *testing.T) {
	path := filepath.Join(t.TempDir(), "identity.json")

	first, err := identity.Generate()
	require.NoError(t, err)
	require.NoError(t, identity.Save(path, first))

	first.Complete = true
	require.NoError(t, identity.Save(path, first))

	got, err := identity.Load(path)
	require.NoError(t, err)
	require.True(t, got.Complete)

	entries, err := os.ReadDir(filepath.Dir(path))
	require.NoError(t, err)
	require.Len(t, entries, 1, "no temp file is left behind")
}

func TestLoad_Missing(t *testing.T) {
	_, err := identity.Load(filepath.Join(t.TempDir(), "identity.json"))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestLoad_Corrupt(t *testing.T) {
	for name, content := range map[string]string{
		"not json":  "garbage",
		"short key": `{"partyId":"p::1","privateKey":"AAAA"}`,
		"no key":    `{"partyId":"p::1"}`,
	} {
		path := filepath.Join(t.TempDir(), "identity.json")
		require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

		_, err := identity.Load(path)
		require.ErrorIs(t, err, identity.ErrCorrupt, name)
	}
}
