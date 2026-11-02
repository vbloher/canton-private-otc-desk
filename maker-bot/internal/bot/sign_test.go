package bot_test

import (
	"crypto/ed25519"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/maker-bot/internal/desk"
)

func Test_SignPending_SignsAllocationsAndAccepts(t *testing.T) {
	allocate := desk.Action{ID: uuid.New(), Purpose: "allocate", Hash: []byte("allocate-hash")}
	accept := desk.Action{ID: uuid.New(), Purpose: "transfer-accept", Hash: []byte("accept-hash")}

	d := &fakeDesk{pending: []desk.Action{allocate, accept}}
	b, id, _ := newBot(t, d)

	b.Event(t.Context(), event(t, "sign.requested", map[string]string{}))

	require.Len(t, d.executions, 2)

	signatures := map[uuid.UUID][]byte{}
	for _, e := range d.executions {
		signatures[e.ID] = e.Signature
	}

	for _, action := range []desk.Action{allocate, accept} {
		require.True(t, ed25519.Verify(id.PublicKey(), action.Hash, signatures[action.ID]), action.Purpose)
	}
}

func Test_SignPending_ExecutesTheActionsAtOnce(t *testing.T) {
	pay := desk.Action{ID: uuid.New(), Purpose: "allocate", Description: "Allocate 66.63 CC", Hash: []byte("pay")}
	receive := desk.Action{ID: uuid.New(), Purpose: "allocate", Description: "Allocate receipt of CBTC", Hash: []byte("receive")}

	together := newMeeting(2)
	d := &fakeDesk{pending: []desk.Action{pay, receive}, executeMeeting: together}
	b, _, _ := newBot(t, d)

	b.Event(t.Context(), event(t, "sign.requested", map[string]string{}))

	require.Len(t, d.executions, 2)
	require.True(t, together.met(), "a quote's allocations commit together, not one after another")
}

func Test_SignPending_RefusesTransferOut(t *testing.T) {
	withdrawal := desk.Action{ID: uuid.New(), Purpose: "transfer-out", Description: "Send 1 CBTC to mallory", Hash: []byte("h")}

	d := &fakeDesk{pending: []desk.Action{withdrawal}}
	b, _, out := newBot(t, d)

	b.Event(t.Context(), event(t, "sign.requested", map[string]string{}))
	b.Event(t.Context(), event(t, "sign.requested", map[string]string{}))

	require.Empty(t, d.executions, "the bot never signs a withdrawal")
	require.Equal(t, 1, out.count(t, "WARN", "transfer-out"), "and says so once")
}

func Test_SignPending_DoesNotResendASentAction(t *testing.T) {
	allocate := desk.Action{ID: uuid.New(), Purpose: "allocate", Hash: []byte("h")}

	d := &fakeDesk{pending: []desk.Action{allocate}}
	b, _, _ := newBot(t, d)

	b.Event(t.Context(), event(t, "sign.requested", map[string]string{}))
	b.Event(t.Context(), event(t, "sign.requested", map[string]string{}))

	require.Len(t, d.executions, 1)
}

func Test_SignPending_RetriesAfterATransientError(t *testing.T) {
	allocate := desk.Action{ID: uuid.New(), Purpose: "allocate", Hash: []byte("h")}

	d := &fakeDesk{
		pending:     []desk.Action{allocate},
		executeErrs: map[uuid.UUID][]error{allocate.ID: {fmt.Errorf("execute: %w", desk.ErrUnavailable)}},
	}
	b, _, _ := newBot(t, d)

	b.Event(t.Context(), event(t, "sign.requested", map[string]string{}))
	b.Event(t.Context(), event(t, "sign.requested", map[string]string{}))

	require.Len(t, d.executions, 2)
}

func Test_SignPending_GoneActionIsNotRetried(t *testing.T) {
	allocate := desk.Action{ID: uuid.New(), Purpose: "allocate", Hash: []byte("h")}

	d := &fakeDesk{
		pending:     []desk.Action{allocate},
		executeErrs: map[uuid.UUID][]error{allocate.ID: {fmt.Errorf("execute: %w", desk.ErrConflict)}},
	}
	b, _, _ := newBot(t, d)

	b.Event(t.Context(), event(t, "sign.requested", map[string]string{}))
	b.Event(t.Context(), event(t, "sign.requested", map[string]string{}))

	require.Len(t, d.executions, 1)
}
