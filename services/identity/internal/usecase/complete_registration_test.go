package usecase_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/dto"
)

// parkedRegistration is a started registration and the browser key's signature over its multi-hash.
func parkedRegistration(t *testing.T) (domain.Registration, []byte) {
	t.Helper()

	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	reg, err := domain.NewRegistration("Alice Smith", publicKey)
	require.NoError(t, err)

	reg.Topology = domain.Topology{
		PartyID:      reg.Hint + "::1220ab",
		Fingerprint:  "1220ab",
		MultiHash:    bytes.Repeat([]byte{9}, 32),
		Transactions: []string{"tx-1", "tx-2"},
	}

	return reg, ed25519.Sign(privateKey, reg.Topology.MultiHash)
}

func completeInput(reg domain.Registration, signature []byte) dto.CompleteRegistrationInput {
	return dto.CompleteRegistrationInput{
		RegistrationID: reg.ID,
		Signature:      signature,
		CredentialID:   "cred-1",
		EncryptedBlob:  "blob-1",
	}
}

func Test_CompleteRegistration_Expired(t *testing.T) {
	d := newDeps(t)

	id := uuid.New()
	d.redis.EXPECT().TakeRegistration(Any, id).Return(domain.Registration{}, domain.ErrRegistrationExpired)

	_, err := d.useCase().CompleteRegistration(t.Context(), dto.CompleteRegistrationInput{
		RegistrationID: id,
		Signature:      make([]byte, 64),
		CredentialID:   "cred-1",
		EncryptedBlob:  "blob-1",
	})
	require.ErrorIs(t, err, domain.ErrRegistrationExpired)
}

func Test_CompleteRegistration_CredentialTaken(t *testing.T) {
	d := newDeps(t)

	reg, signature := parkedRegistration(t)

	d.redis.EXPECT().TakeRegistration(Any, reg.ID).Return(reg, nil)
	d.postgres.EXPECT().GetUserByCredential(Any, "cred-1").Return(domain.User{ID: uuid.New()}, nil)
	// No ledger expectation: allocating the party before the credential check would fail the test.

	_, err := d.useCase().CompleteRegistration(t.Context(), completeInput(reg, signature))
	require.ErrorIs(t, err, domain.ErrCredentialTaken)
}

func Test_CompleteRegistration_CreatesUserAndSession(t *testing.T) {
	d := newDeps(t)

	reg, signature := parkedRegistration(t)

	var (
		calls         []string
		created       domain.User
		sessionHash   string
		sessionUserID uuid.UUID
		sessionExpiry time.Time
	)

	d.redis.EXPECT().TakeRegistration(Any, reg.ID).
		Run(func(context.Context, uuid.UUID) { calls = append(calls, "take registration") }).
		Return(reg, nil)
	d.postgres.EXPECT().GetUserByCredential(Any, "cred-1").
		Run(func(context.Context, string) { calls = append(calls, "check credential") }).
		Return(domain.User{}, domain.ErrNotFound)
	d.ledger.EXPECT().AllocateExternalParty(Any, reg.Topology, signature).
		Run(func(context.Context, domain.Topology, []byte) { calls = append(calls, "allocate party") }).
		Return(reg.Topology.PartyID, nil)
	d.postgres.EXPECT().CreateUser(Any, Any).
		Run(func(_ context.Context, u domain.User) { calls, created = append(calls, "create user"), u }).
		Return(nil)
	d.postgres.EXPECT().CreateSession(Any, Any, Any, Any).
		Run(func(_ context.Context, hash string, userID uuid.UUID, expiresAt time.Time) {
			calls = append(calls, "create session")
			sessionHash, sessionUserID, sessionExpiry = hash, userID, expiresAt
		}).
		Return(nil)

	before := time.Now()

	output, err := d.useCase().CompleteRegistration(t.Context(), completeInput(reg, signature))
	require.NoError(t, err)

	after := time.Now()

	require.Equal(t,
		[]string{"take registration", "check credential", "allocate party", "create user", "create session"},
		calls, "the credential is checked before the ledger is called, the user exists before its session")

	require.Equal(t, domain.KindPasskey, created.Kind)
	require.Equal(t, reg.Topology.PartyID, created.PartyID)
	require.Equal(t, reg.Hint, created.Hint)
	require.Equal(t, "Alice Smith", created.DisplayName)
	require.Equal(t, reg.PublicKey, created.PublicKey)
	require.Equal(t, "cred-1", created.CredentialID)
	require.Equal(t, "blob-1", created.EncryptedBlob)
	require.False(t, created.MakerMode)

	require.Equal(t, created, output.User)
	require.NotEmpty(t, output.Token)
	require.Equal(t, domain.HashSessionToken(output.Token), sessionHash, "only the hash of the token is stored")
	require.Equal(t, created.ID, sessionUserID)
	require.Equal(t, testConfig.SessionTTL, output.TTL)
	require.WithinRange(t, sessionExpiry, before.Add(testConfig.SessionTTL), after.Add(testConfig.SessionTTL))
}

func Test_CompleteRegistration_BadSignature(t *testing.T) {
	d := newDeps(t)

	reg, _ := parkedRegistration(t)

	// A well-formed signature, made by another key.
	_, otherKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	forged := ed25519.Sign(otherKey, reg.Topology.MultiHash)

	d.redis.EXPECT().TakeRegistration(Any, reg.ID).Return(reg, nil)
	// No ledger or user expectation: a forged onboarding signature never reaches Canton.

	_, err = d.useCase().CompleteRegistration(t.Context(), completeInput(reg, forged))
	require.ErrorIs(t, err, domain.ErrValidation)
	require.ErrorContains(t, err, "signature")
}

func Test_CompleteRegistration_MalformedSignatureKeepsTheRegistration(t *testing.T) {
	d := newDeps(t) // no expectations: the registration must not be taken

	reg, signature := parkedRegistration(t)

	_, err := d.useCase().CompleteRegistration(t.Context(), completeInput(reg, signature[:63]))
	require.ErrorIs(t, err, domain.ErrValidation)
}

func Test_CompleteRegistration_LedgerFails(t *testing.T) {
	d := newDeps(t)

	reg, signature := parkedRegistration(t)

	d.redis.EXPECT().TakeRegistration(Any, reg.ID).Return(reg, nil)
	d.postgres.EXPECT().GetUserByCredential(Any, "cred-1").Return(domain.User{}, domain.ErrNotFound)
	d.ledger.EXPECT().AllocateExternalParty(Any, reg.Topology, signature).Return("", domain.ErrLedger)

	_, err := d.useCase().CompleteRegistration(t.Context(), completeInput(reg, signature))
	require.ErrorIs(t, err, domain.ErrLedger)
}

func Test_CompleteRegistration_CredentialTakenByARace(t *testing.T) {
	d := newDeps(t)

	reg, signature := parkedRegistration(t)

	d.redis.EXPECT().TakeRegistration(Any, reg.ID).Return(reg, nil)
	d.postgres.EXPECT().GetUserByCredential(Any, "cred-1").Return(domain.User{}, domain.ErrNotFound)
	d.ledger.EXPECT().AllocateExternalParty(Any, reg.Topology, signature).Return(reg.Topology.PartyID, nil)
	d.postgres.EXPECT().CreateUser(Any, Any).Return(domain.ErrCredentialTaken)
	// No CreateSession expectation: no session without a user.

	_, err := d.useCase().CompleteRegistration(t.Context(), completeInput(reg, signature))
	require.ErrorIs(t, err, domain.ErrCredentialTaken)
}

// A party that Canton allocated for input that cannot become a user is orphaned for good, so
// nothing may reach the ledger before the credential id and the wrapped key are known to be there.
func Test_CompleteRegistration_EmptyCredentialOrBlobNeverReachesTheLedger(t *testing.T) {
	reg, signature := parkedRegistration(t)

	cases := map[string]dto.CompleteRegistrationInput{
		"no credential id": {RegistrationID: reg.ID, Signature: signature, CredentialID: "", EncryptedBlob: "blob-1"},
		"no wrapped key":   {RegistrationID: reg.ID, Signature: signature, CredentialID: "cred-1", EncryptedBlob: ""},
		"neither":          {RegistrationID: reg.ID, Signature: signature, CredentialID: "", EncryptedBlob: ""},
	}

	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			// Strict mocks without a single expectation: not the ledger, not Postgres, and not
			// Redis either, so the parked registration is not burned by a request that can only fail.
			d := newDeps(t)

			_, err := d.useCase().CompleteRegistration(t.Context(), input)
			require.ErrorIs(t, err, domain.ErrValidation)
		})
	}
}
