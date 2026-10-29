package usecase_test

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/identity/internal/dto"
)

func Test_StartRegistration_ParksTopology(t *testing.T) {
	d := newDeps(t)

	publicKey := bytes.Repeat([]byte{1}, 32)
	topology := domain.Topology{
		PartyID:      "alice-smith-3f9a1c::1220ab",
		Fingerprint:  "1220ab",
		MultiHash:    []byte("multi-hash"),
		Transactions: []string{"tx-1", "tx-2"},
	}

	var hint string

	d.ledger.EXPECT().GenerateExternalParty(Any, Any, publicKey).
		Run(func(_ context.Context, h string, _ []byte) { hint = h }).
		Return(topology, nil)

	var (
		parked    domain.Registration
		parkedTTL time.Duration
	)

	d.redis.EXPECT().SaveRegistration(Any, Any, Any).
		Run(func(_ context.Context, r domain.Registration, ttl time.Duration) { parked, parkedTTL = r, ttl }).
		Return(nil)

	output, err := d.useCase().StartRegistration(t.Context(), dto.StartRegistrationInput{
		DisplayName: "  Alice Smith ",
		PublicKey:   publicKey,
	})
	require.NoError(t, err)

	require.NotEqual(t, uuid.Nil, parked.ID)
	require.Equal(t, dto.StartRegistrationOutput{
		RegistrationID: parked.ID,
		PartyID:        topology.PartyID,
		MultiHash:      topology.MultiHash,
	}, output)

	require.Regexp(t, `^alice-smith-[0-9a-f]{6}$`, hint)
	require.Equal(t, hint, parked.Hint, "the hint the ledger got is the hint that is parked")
	require.Equal(t, "Alice Smith", parked.DisplayName)
	require.Equal(t, publicKey, parked.PublicKey)
	require.Equal(t, topology, parked.Topology, "the whole topology is parked for the completion")
	require.Equal(t, 10*time.Minute, parkedTTL)
}

func Test_StartRegistration_InvalidInput(t *testing.T) {
	for name, input := range map[string]dto.StartRegistrationInput{
		"one rune name": {DisplayName: "A", PublicKey: bytes.Repeat([]byte{1}, 32)},
		"short key":     {DisplayName: "Alice", PublicKey: bytes.Repeat([]byte{1}, 31)},
	} {
		d := newDeps(t)

		_, err := d.useCase().StartRegistration(t.Context(), input)
		require.ErrorIs(t, err, domain.ErrValidation, name)
	}
}

func Test_StartRegistration_LedgerFails(t *testing.T) {
	d := newDeps(t)

	d.ledger.EXPECT().GenerateExternalParty(Any, Any, Any).Return(domain.Topology{}, domain.ErrLedger)

	_, err := d.useCase().StartRegistration(t.Context(), dto.StartRegistrationInput{
		DisplayName: "Alice",
		PublicKey:   bytes.Repeat([]byte{1}, 32),
	})
	require.ErrorIs(t, err, domain.ErrLedger)
}
