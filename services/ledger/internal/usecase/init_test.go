package usecase_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/usecase"
)

func Test_Init_WaitsForParties(t *testing.T) {
	d := newDeps(t)

	// /readyz turns green before the seed script has allocated the parties.
	d.canton.EXPECT().Ready(Any).Return(false).Once()
	d.canton.EXPECT().Ready(Any).Return(true).Once()
	d.canton.EXPECT().ResolveParty(Any, "operator").Return("", domain.ErrNotFound).Once()
	d.canton.EXPECT().ResolveParty(Any, "operator").Return(operator, nil).Once()
	d.canton.EXPECT().ResolveParty(Any, "makerA").Return(makerA, nil).Once()
	d.canton.EXPECT().ResolveParty(Any, "makerB").Return(makerB, nil).Once()
	d.expectWarmUp()

	u := d.useCase(t)

	require.NoError(t, u.Init(t.Context()))

	desk := u.GetDesk(t.Context())
	require.Equal(t, operator, desk.OperatorPartyID)
	require.Len(t, desk.Instruments, 3)
}

func Test_Init_GivesUpOnMissingParties(t *testing.T) {
	d := newDeps(t)

	d.canton.EXPECT().Ready(Any).Return(true).Once()
	d.canton.EXPECT().ResolveParty(Any, "operator").Return(operator, nil).Once()
	d.canton.EXPECT().ResolveParty(Any, "makerA").Return(makerA, nil).Once()
	d.canton.EXPECT().ResolveParty(Any, "makerB").Return("", domain.ErrNotFound)

	c := config(t)
	c.StartupTimeout = 50 * time.Millisecond

	u := usecase.New(c, d.tx, d.postgres, d.redis, d.canton, d.events)

	err := u.Init(t.Context())
	require.ErrorContains(t, err, "makerB")
}

func Test_Init_FailsAtOnceOnAnAmbiguousParty(t *testing.T) {
	d := newDeps(t)

	d.canton.EXPECT().Ready(Any).Return(true).Once()
	d.canton.EXPECT().ResolveParty(Any, "operator").Return(operator, nil).Once()
	d.canton.EXPECT().ResolveParty(Any, "makerA").
		Return("", fmt.Errorf("%w: %q names makerA::1220aa, makerA::1220bb", domain.ErrAmbiguousParty, "makerA")).Once()
	d.canton.EXPECT().ResolveParty(Any, "makerB").Return(makerB, nil).Maybe()

	// The ledger was seeded twice: waiting cannot fix that.
	start := time.Now()
	err := d.useCase(t).Init(t.Context())

	require.ErrorIs(t, err, domain.ErrAmbiguousParty)
	require.ErrorContains(t, err, "makerA::1220aa, makerA::1220bb")
	require.Less(t, time.Since(start), time.Second)
}
