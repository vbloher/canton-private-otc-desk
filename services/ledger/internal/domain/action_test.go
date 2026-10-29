package domain_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/domain"
)

func Test_Action_Duplicates(t *testing.T) {
	allocate := domain.Intent{
		PartyID: "alice", Purpose: domain.PurposeAllocate, Ref: "s-1",
		Description: "Allocate 1.0 CBTC for quote s-1",
	}
	action := domain.NewAction(allocate, domain.Prepared{Hash: []byte("h")}, time.Now(), time.Minute)

	require.True(t, action.Duplicates(allocate))
	require.True(t, action.Duplicates(domain.Intent{
		PartyID: "alice", Purpose: domain.PurposeAllocate, Ref: "s-1",
		Description: "Allocate receipt of USDCx for quote s-1",
	}))
	require.False(t, action.Duplicates(domain.Intent{PartyID: "bob", Purpose: domain.PurposeAllocate, Ref: "s-1"}))
	require.False(t, action.Duplicates(domain.Intent{PartyID: "alice", Purpose: domain.PurposeAllocate, Ref: "s-2"}))

	withdraw := domain.Intent{
		PartyID: "alice", Purpose: domain.PurposeTransferOut, Ref: "bob::1220",
		Description: "Withdraw 1.0 CBTC to bob", Counterparty: "bob::1220",
	}
	pending := domain.NewAction(withdraw, domain.Prepared{}, time.Now(), time.Minute)

	require.True(t, pending.Duplicates(withdraw))
	require.False(t, pending.Duplicates(domain.Intent{
		PartyID: "alice", Purpose: domain.PurposeTransferOut,
		Ref: "bob::1220", Description: "Withdraw 2.0 CETH to bob",
	}))
	require.Equal(t, "bob::1220", pending.Counterparty)
}

func Test_Invalidf_KeepsTheReason(t *testing.T) {
	err := domain.Invalidf("amount %q is not a positive decimal", "0")

	require.ErrorIs(t, err, domain.ErrValidation)
	require.True(t, domain.IsInvalidInput(err))
	require.EqualError(t, err, `validation failed: amount "0" is not a positive decimal`)
	require.NoError(t, errorsUnwrap(err))
}

func Test_Action_Outcome(t *testing.T) {
	pending := domain.NewAction(domain.Intent{PartyID: "alice", Purpose: domain.PurposeAllocate, Ref: "s-1"},
		domain.Prepared{}, time.Now(), time.Minute)

	executed, record := pending.Outcome(nil)
	require.True(t, record)
	require.Equal(t, domain.ActionExecuted, executed.Status)

	duplicate, record := pending.Outcome(fmt.Errorf("%w: %w", domain.ErrLedger, domain.ErrDuplicateCommand))
	require.True(t, record)
	require.Equal(t, domain.ActionExecuted, duplicate.Status)

	unchanged, record := pending.Outcome(fmt.Errorf("%w: %w", domain.ErrLedger, domain.ErrLedgerUnavailable))
	require.False(t, record)
	require.Equal(t, pending, unchanged)

	wrongKey, record := pending.Outcome(fmt.Errorf("%w: FAILED_TO_EXECUTE_TRANSACTION", domain.ErrSignatureRejected))
	require.False(t, record)
	require.Equal(t, pending, wrongKey)

	rejected := fmt.Errorf("%w: FAILED_TO_EXECUTE_TRANSACTION: interpretation failed", domain.ErrLedger)
	failed, record := pending.Outcome(rejected)
	require.True(t, record)
	require.Equal(t, domain.ActionFailed, failed.Status)
	require.Equal(t, rejected.Error(), failed.Error)
}

func Test_Errorf_KeepsTheReason(t *testing.T) {
	err := domain.Errorf(domain.ErrNotFound, "no pending transfer %s", "offer-1")

	require.ErrorIs(t, err, domain.ErrNotFound)
	require.EqualError(t, err, "not found: no pending transfer offer-1")
	require.NoError(t, errorsUnwrap(err))
}
