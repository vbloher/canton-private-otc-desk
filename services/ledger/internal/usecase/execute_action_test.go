package usecase_test

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/pkg/events"
	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/dto"
)

//nolint:gochecknoglobals
var signature = bytes.Repeat([]byte{7}, 64)

func pendingAction(createdAt time.Time) domain.Action {
	return domain.NewAction(domain.Intent{
		PartyID: alice, Purpose: domain.PurposeAllocate, Ref: "s-1",
		Description: "Allocate 1.0 CBTC",
	}, prepared("hash-1"), createdAt, 10*time.Minute)
}

func live() any {
	return mock.MatchedBy(func(ctx context.Context) bool { return ctx.Err() == nil })
}

func Test_ExecuteAction_Success(t *testing.T) {
	d := newDeps(t)
	u := d.started(t)

	action := pendingAction(time.Now())
	actionEvent := fmt.Sprintf(`{"actionId":%q}`, action.ID)

	d.postgres.EXPECT().GetActionForUpdate(Any, action.ID).Return(action, nil).Once()
	d.canton.EXPECT().Execute(Any, action.Prepared, alice, signature).Return(nil).Once()
	d.postgres.EXPECT().UpdateAction(Any, mock.MatchedBy(func(a domain.Action) bool {
		return a.ID == action.ID && a.Status == domain.ActionExecuted && a.Error == ""
	})).Return(nil).Once()
	d.events.EXPECT().Publish(Any, event(events.SignCompleted, actionEvent), parties(alice)).Return(nil).Once()
	d.events.EXPECT().Publish(Any, event(events.WalletUpdated, `{}`), parties(alice)).Return(nil).Once()

	err := u.ExecuteAction(t.Context(), dto.ExecuteActionInput{ID: action.ID, PartyID: alice, Signature: signature})
	require.NoError(t, err)
}

func Test_ExecuteAction_LedgerFailureMarksFailed(t *testing.T) {
	d := newDeps(t)
	u := d.started(t)

	action := pendingAction(time.Now())
	ledgerErr := fmt.Errorf("%w: FAILED_TO_EXECUTE_TRANSACTION: the prepared transaction is malformed", domain.ErrLedger)

	d.postgres.EXPECT().GetActionForUpdate(Any, action.ID).Return(action, nil).Once()
	d.canton.EXPECT().Execute(Any, action.Prepared, alice, signature).Return(ledgerErr).Once()
	d.postgres.EXPECT().UpdateAction(Any, mock.MatchedBy(func(a domain.Action) bool {
		return a.Status == domain.ActionFailed && a.Error == ledgerErr.Error()
	})).Return(nil).Once()
	d.events.EXPECT().Publish(Any, eventOfType(events.SignCompleted), parties(alice)).Return(nil).Once()

	err := u.ExecuteAction(t.Context(), dto.ExecuteActionInput{ID: action.ID, PartyID: alice, Signature: signature})
	require.ErrorIs(t, err, domain.ErrLedger)
}

func Test_ExecuteAction_DuplicateCommandIsExecuted(t *testing.T) {
	d := newDeps(t)
	u := d.started(t)

	action := pendingAction(time.Now())

	d.postgres.EXPECT().GetActionForUpdate(Any, action.ID).Return(action, nil).Once()
	d.canton.EXPECT().Execute(Any, action.Prepared, alice, signature).
		Return(fmt.Errorf("%w: %w: DUPLICATE_COMMAND", domain.ErrLedger, domain.ErrDuplicateCommand)).Once()
	d.postgres.EXPECT().UpdateAction(Any, mock.MatchedBy(func(a domain.Action) bool {
		return a.Status == domain.ActionExecuted && a.Error == ""
	})).Return(nil).Once()
	d.events.EXPECT().Publish(Any, eventOfType(events.SignCompleted), parties(alice)).Return(nil).Once()
	d.events.EXPECT().Publish(Any, eventOfType(events.WalletUpdated), parties(alice)).Return(nil).Once()

	err := u.ExecuteAction(t.Context(), dto.ExecuteActionInput{ID: action.ID, PartyID: alice, Signature: signature})
	require.NoError(t, err)
}

func Test_ExecuteAction_TransientFailureKeepsItPending(t *testing.T) {
	d := newDeps(t)
	u := d.started(t)

	action := pendingAction(time.Now())

	d.postgres.EXPECT().GetActionForUpdate(Any, action.ID).Return(action, nil).Once()
	d.canton.EXPECT().Execute(Any, action.Prepared, alice, signature).
		Return(fmt.Errorf("%w: %w: connection refused", domain.ErrLedger, domain.ErrLedgerUnavailable)).Once()

	err := u.ExecuteAction(t.Context(), dto.ExecuteActionInput{ID: action.ID, PartyID: alice, Signature: signature})
	require.ErrorIs(t, err, domain.ErrLedgerUnavailable)
}

func Test_ExecuteAction_WrongKeyKeepsItPending(t *testing.T) {
	d := newDeps(t)
	u := d.started(t)

	action := pendingAction(time.Now())

	d.postgres.EXPECT().GetActionForUpdate(Any, action.ID).Return(action, nil).Once()
	d.canton.EXPECT().Execute(Any, action.Prepared, alice, signature).
		Return(fmt.Errorf("%w: FAILED_TO_EXECUTE_TRANSACTION: Received 0 valid signatures", domain.ErrSignatureRejected)).Once()

	err := u.ExecuteAction(t.Context(), dto.ExecuteActionInput{ID: action.ID, PartyID: alice, Signature: signature})
	require.ErrorIs(t, err, domain.ErrSignatureRejected)
	require.True(t, domain.IsInvalidInput(err))
}

func Test_ExecuteAction_ContractGoneFailsIt(t *testing.T) {
	d := newDeps(t)
	u := d.started(t)

	action := pendingAction(time.Now())

	d.postgres.EXPECT().GetActionForUpdate(Any, action.ID).Return(action, nil).Once()
	d.canton.EXPECT().Execute(Any, action.Prepared, alice, signature).
		Return(fmt.Errorf("%w: %w: LOCAL_VERDICT_INACTIVE_CONTRACTS", domain.ErrLedger, domain.ErrContractGone)).Once()
	d.postgres.EXPECT().UpdateAction(Any, mock.MatchedBy(func(a domain.Action) bool {
		return a.Status == domain.ActionFailed
	})).Return(nil).Once()
	d.events.EXPECT().Publish(Any, eventOfType(events.SignCompleted), parties(alice)).Return(nil).Once()

	err := u.ExecuteAction(t.Context(), dto.ExecuteActionInput{ID: action.ID, PartyID: alice, Signature: signature})
	require.ErrorIs(t, err, domain.ErrContractGone)
}

func Test_ExecuteAction_OutlivesTheCaller(t *testing.T) {
	d := newDeps(t)
	u := d.started(t)

	withdrawal := domain.NewAction(domain.Intent{
		PartyID: alice, Purpose: domain.PurposeTransferOut, Ref: bob,
		Counterparty: bob, Description: "Withdraw 1.0 CBTC to bob",
	}, prepared("hash-w"), time.Now(), time.Minute)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	d.postgres.EXPECT().GetActionForUpdate(live(), withdrawal.ID).Return(withdrawal, nil).Once()
	d.canton.EXPECT().Execute(live(), withdrawal.Prepared, alice, signature).Return(nil).Once()
	d.postgres.EXPECT().UpdateAction(live(), mock.MatchedBy(func(a domain.Action) bool {
		return a.Status == domain.ActionExecuted
	})).Return(nil).Once()
	d.events.EXPECT().Publish(Any, eventOfType(events.SignCompleted), parties(alice)).Return(nil).Once()
	d.events.EXPECT().Publish(Any, eventOfType(events.WalletUpdated), parties(alice, bob)).Return(nil).Once()

	err := u.ExecuteAction(ctx, dto.ExecuteActionInput{ID: withdrawal.ID, PartyID: alice, Signature: signature})
	require.NoError(t, err)
}

func Test_ExecuteAction_WrongParty(t *testing.T) {
	d := newDeps(t)
	u := d.started(t)

	action := pendingAction(time.Now())

	d.postgres.EXPECT().GetActionForUpdate(Any, action.ID).Return(action, nil).Once()

	err := u.ExecuteAction(t.Context(), dto.ExecuteActionInput{ID: action.ID, PartyID: maker, Signature: signature})
	require.ErrorIs(t, err, domain.ErrNotFound)
}

func Test_ExecuteAction_Expired(t *testing.T) {
	d := newDeps(t)
	u := d.started(t)

	action := pendingAction(time.Now().Add(-11 * time.Minute))

	d.postgres.EXPECT().GetActionForUpdate(Any, action.ID).Return(action, nil).Once()

	err := u.ExecuteAction(t.Context(), dto.ExecuteActionInput{ID: action.ID, PartyID: alice, Signature: signature})
	require.ErrorIs(t, err, domain.ErrActionNotPending)
}

func Test_ExecuteAction_AlreadyExecuted(t *testing.T) {
	d := newDeps(t)
	u := d.started(t)

	action := pendingAction(time.Now()).Finished(nil)

	d.postgres.EXPECT().GetActionForUpdate(Any, action.ID).Return(action, nil).Once()

	err := u.ExecuteAction(t.Context(), dto.ExecuteActionInput{ID: action.ID, PartyID: alice, Signature: signature})
	require.ErrorIs(t, err, domain.ErrActionNotPending)
}

func Test_ExecuteAction_SignatureLength(t *testing.T) {
	d := newDeps(t)
	u := d.started(t)

	action := pendingAction(time.Now())

	err := u.ExecuteAction(t.Context(), dto.ExecuteActionInput{ID: action.ID, PartyID: alice, Signature: []byte{1, 2}})
	require.ErrorIs(t, err, domain.ErrValidation)
}

func Test_ExpireActions_NotifiesParties(t *testing.T) {
	d := newDeps(t)
	u := d.started(t)

	expired := pendingAction(time.Now().Add(-time.Hour))

	d.postgres.EXPECT().ExpireActions(Any, mock.AnythingOfType("time.Time")).Return([]domain.Action{expired}, nil).Once()
	d.events.EXPECT().Publish(Any, event(events.SignCompleted, fmt.Sprintf(`{"actionId":%q}`, expired.ID)),
		parties(alice)).Return(nil).Once()

	output, err := u.ExpireActions(t.Context())
	require.NoError(t, err)
	require.Equal(t, 1, output.Expired)
}

func Test_ExpireActions_RecordsNoSpanWhenNothingExpired(t *testing.T) {
	d := newDeps(t)
	u := d.started(t)
	recorder := recordSpans(t)

	d.postgres.EXPECT().ExpireActions(Any, mock.AnythingOfType("time.Time")).Return(nil, nil).Once()

	output, err := u.ExpireActions(tracer.Suppress(t.Context()))
	require.NoError(t, err)
	require.Equal(t, 0, output.Expired)
	require.Empty(t, recorder.Ended())
}

func Test_ExpireActions_TracesExpiredActionsAsARoot(t *testing.T) {
	d := newDeps(t)
	u := d.started(t)
	recorder := recordSpans(t)

	expired := pendingAction(time.Now().Add(-time.Hour))

	d.postgres.EXPECT().ExpireActions(Any, mock.AnythingOfType("time.Time")).Return([]domain.Action{expired}, nil).Once()
	d.events.EXPECT().Publish(Any, event(events.SignCompleted, fmt.Sprintf(`{"actionId":%q}`, expired.ID)),
		parties(alice)).Return(nil).Once()

	_, err := u.ExpireActions(tracer.Suppress(t.Context()))
	require.NoError(t, err)

	spans := recorder.Ended()
	root := roots(spans)
	require.Len(t, root, 1)
	require.Equal(t, "usecase ExpireActions", root[0].Name())

	for _, s := range spans {
		require.Equal(t, root[0].SpanContext().TraceID(), s.SpanContext().TraceID())
	}
}
