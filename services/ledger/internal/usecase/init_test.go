package usecase_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/usecase"
)

func Test_Init_WaitsForParties(t *testing.T) {
	d := newDeps(t)

	d.canton.EXPECT().Ready(Any).Return(false).Once()
	d.canton.EXPECT().Ready(Any).Return(true).Once()
	d.canton.EXPECT().ResolveParty(Any, operator).Return("", domain.ErrNotFound).Once()
	d.canton.EXPECT().ResolveParty(Any, operator).Return(operator, nil).Once()
	d.canton.EXPECT().ResolveParty(Any, maker).Return(maker, nil).Once()
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
	d.canton.EXPECT().ResolveParty(Any, operator).Return(operator, nil).Once()
	d.canton.EXPECT().ResolveParty(Any, maker).Return("", domain.ErrNotFound)

	c := config(t)
	c.StartupTimeout = 50 * time.Millisecond

	u := usecase.New(c, d.tx, d.postgres, d.redis, d.canton, d.registry, d.events)

	err := u.Init(t.Context())
	require.ErrorContains(t, err, maker)
}

func Test_Init_WithoutAMaker(t *testing.T) {
	d := newDeps(t)

	d.canton.EXPECT().Ready(Any).Return(true).Once()
	d.canton.EXPECT().ResolveParty(Any, operator).Return(operator, nil).Once()
	d.expectWarmUp()

	c := config(t)
	c.MakerParty = ""

	u := usecase.New(c, d.tx, d.postgres, d.redis, d.canton, d.registry, d.events)
	require.NoError(t, u.Init(t.Context()))
}

func Test_Init_RejectsAHint(t *testing.T) {
	d := newDeps(t)

	d.canton.EXPECT().Ready(Any).Return(true).Once()

	c := config(t)
	c.OperatorParty = "otc-desk-operator"

	u := usecase.New(c, d.tx, d.postgres, d.redis, d.canton, d.registry, d.events)

	err := u.Init(t.Context())
	require.ErrorIs(t, err, domain.ErrValidation)
	require.ErrorContains(t, err, `"otc-desk-operator" is not a full party id`)
}
