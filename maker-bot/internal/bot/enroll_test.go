package bot_test

import (
	"crypto/ed25519"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/maker-bot/internal/bot"
	"github.com/vbloher/canton-private-otc-desk/maker-bot/internal/desk"
	"github.com/vbloher/canton-private-otc-desk/maker-bot/internal/identity"
)

const botParty = "otc-demo-maker-1a2b3c::1220aa"

func identityPath(t *testing.T) string {
	t.Helper()

	return filepath.Join(t.TempDir(), "state", "identity.json")
}

func registeringDesk() *fakeDesk {
	return &fakeDesk{
		registration: desk.Registration{ID: uuid.New(), PartyID: botParty, MultiHash: []byte("multi-hash")},
		me:           desk.Me{PartyID: botParty, Hint: "otc-demo-maker-1a2b3c", Kind: "key"},
	}
}

func savedIdentity(t *testing.T, path string, complete bool, registrationID uuid.UUID) identity.Identity {
	t.Helper()

	id, err := identity.Generate()
	require.NoError(t, err)

	id.PartyID, id.RegistrationID, id.MultiHash, id.Complete = botParty, registrationID, []byte("multi-hash"), complete
	if complete {
		id.Hint = "otc-demo-maker-1a2b3c"
	}

	require.NoError(t, identity.Save(path, id))

	return id
}

func TestEnroll_FirstRun(t *testing.T) {
	d := registeringDesk()
	path := identityPath(t)

	id, err := bot.Enroll(t.Context(), d, path, "OTC Demo Maker", quiet)
	require.NoError(t, err)

	require.Equal(t, []string{"StartRegistration", "SetSigner", "RegisterParty", "CompleteRegistration"}, d.callNames())
	require.Equal(t, botParty, id.PartyID)
	require.Equal(t, "otc-demo-maker-1a2b3c", id.Hint)
	require.True(t, id.Complete)
	require.Equal(t, "OTC Demo Maker", d.displayName)
	require.Equal(t, botParty, d.signerParty)

	saved, err := identity.Load(path)
	require.NoError(t, err)
	require.True(t, saved.Complete)
	require.Equal(t, "otc-demo-maker-1a2b3c", saved.Hint)
	require.Equal(t, d.publicKey, saved.PublicKey(), "the desk registered the key that is on disk")
	require.True(t, ed25519.Verify(saved.PublicKey(), []byte("multi-hash"), d.registerSignature))

	info, err := os.Stat(path)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func TestEnroll_LogsTheRegistration(t *testing.T) {
	d := registeringDesk()
	out := &logs{}

	_, err := bot.Enroll(t.Context(), d, identityPath(t), "OTC Demo Maker",
		slog.New(slog.NewJSONHandler(out, nil)))
	require.NoError(t, err)

	records := out.records(t)
	require.NotEmpty(t, records)

	last := records[len(records)-1]
	require.Equal(t, "registered a new party", last["msg"])
	require.Equal(t, botParty, last["party"])
	require.Equal(t, "otc-demo-maker-1a2b3c", last["hint"])
}

func TestEnroll_LogsARestart(t *testing.T) {
	d := registeringDesk()
	path := identityPath(t)
	savedIdentity(t, path, false, uuid.New())

	out := &logs{}

	_, err := bot.Enroll(t.Context(), d, path, "OTC Demo Maker", slog.New(slog.NewJSONHandler(out, nil)))
	require.NoError(t, err)
	require.Equal(t, 1, out.count(t, "INFO", "the registration expired"))
}

func TestEnroll_SavesTheKeyBeforeThePartyExists(t *testing.T) {
	d := registeringDesk()
	path := identityPath(t)

	d.onRegister = func() {
		saved, err := identity.Load(path)
		require.NoError(t, err)
		require.False(t, saved.Complete)
		require.Equal(t, d.publicKey, saved.PublicKey())
	}

	_, err := bot.Enroll(t.Context(), d, path, "OTC Demo Maker", quiet)
	require.NoError(t, err)
}

func TestEnroll_ResumesAnUnfinishedRegistration(t *testing.T) {
	d := registeringDesk()
	path := identityPath(t)
	saved := savedIdentity(t, path, false, d.registration.ID)

	id, err := bot.Enroll(t.Context(), d, path, "OTC Demo Maker", quiet)
	require.NoError(t, err)

	require.Equal(t, []string{"SetSigner", "RegisterParty", "CompleteRegistration"}, d.callNames())
	require.True(t, id.Complete)
	require.Equal(t, saved.PublicKey(), id.PublicKey())
}

func TestEnroll_StartsOverWhenTheRegistrationIsGone(t *testing.T) {
	d := registeringDesk()
	path := identityPath(t)
	saved := savedIdentity(t, path, false, uuid.New())

	id, err := bot.Enroll(t.Context(), d, path, "OTC Demo Maker", quiet)
	require.NoError(t, err)

	require.Equal(t, []string{
		"SetSigner", "RegisterParty", "StartRegistration", "SetSigner", "RegisterParty", "CompleteRegistration",
	}, d.callNames())
	require.Equal(t, saved.PublicKey(), d.publicKey, "the restart keeps the key")
	require.Equal(t, d.registration.ID, id.RegistrationID)
	require.True(t, id.Complete)
}

func TestEnroll_GivesUpWhenTheRestartIsGoneToo(t *testing.T) {
	d := registeringDesk()
	d.registerErrs = []error{desk.ErrGone, desk.ErrGone}
	path := identityPath(t)
	savedIdentity(t, path, false, uuid.New())

	_, err := bot.Enroll(t.Context(), d, path, "OTC Demo Maker", quiet)
	require.ErrorIs(t, err, desk.ErrGone)
}

func TestEnroll_SignsInWithACompleteIdentity(t *testing.T) {
	d := registeringDesk()
	path := identityPath(t)
	saved := savedIdentity(t, path, true, uuid.New())

	id, err := bot.Enroll(t.Context(), d, path, "OTC Demo Maker", quiet)
	require.NoError(t, err)

	require.Equal(t, []string{"SetSigner", "SignIn"}, d.callNames())
	require.Equal(t, botParty, d.signerParty)
	require.Equal(t, saved.PublicKey(), id.PublicKey())
}

func TestEnroll_UnknownPartyStops(t *testing.T) {
	d := registeringDesk()
	d.signInErr = fmt.Errorf("challenge: %w", desk.ErrNotFound)
	path := identityPath(t)
	savedIdentity(t, path, true, uuid.New())

	before, err := os.ReadFile(path)
	require.NoError(t, err)

	_, err = bot.Enroll(t.Context(), d, path, "OTC Demo Maker", quiet)
	require.ErrorIs(t, err, bot.ErrUnknownParty)
	require.ErrorIs(t, err, desk.ErrNotFound, "the desk's own answer stays in the chain")
	require.ErrorContains(t, err, "challenge: not found")
	require.ErrorContains(t, err, botParty)

	after, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, before, after, "a party the desk does not know is never replaced")
}

func TestEnroll_CorruptFileStops(t *testing.T) {
	d := registeringDesk()
	path := identityPath(t)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, []byte("garbage"), 0o600))

	_, err := bot.Enroll(t.Context(), d, path, "OTC Demo Maker", quiet)
	require.ErrorIs(t, err, identity.ErrCorrupt)
	require.Empty(t, d.callNames())

	after, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "garbage", string(after))
}
