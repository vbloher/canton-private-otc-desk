package usecase_test

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/pkg/decimal"
	"github.com/vbloher/canton-private-otc-desk/pkg/events"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/dto"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/usecase"
)

func faucetConfig(t *testing.T) usecase.Config {
	t.Helper()

	c := config(t)
	c.FaucetParty = faucet

	return c
}

func (d deps) startedWithFaucet(t *testing.T, c usecase.Config) *usecase.UseCase {
	t.Helper()

	d.canton.EXPECT().Ready(Any).Return(true).Once()
	d.canton.EXPECT().ResolveParty(Any, operator).Return(operator, nil).Once()

	if c.Network == domain.NetworkDevNet {
		d.canton.EXPECT().ResolveParty(Any, faucet).Return(faucet, nil).Once()
	}

	d.expectWarmUp()

	u := usecase.New(c, d.tx, d.postgres, d.redis, d.canton, d.registry, d.events)
	require.NoError(t, u.Init(t.Context()))

	return u
}

func faucetCommandID() any {
	return mock.MatchedBy(func(id string) bool { return strings.HasPrefix(id, "faucet:") })
}

func dripSpec(instrument, amount, input string) any {
	return mock.MatchedBy(func(s domain.TransferSpec) bool {
		return s.Registry == registryURL && s.Admin == operator && s.Sender == faucet && s.Receiver == alice &&
			s.InstrumentID == instrument && s.Amount.String() == amount && len(s.Inputs) == 1 && s.Inputs[0] == input
	})
}

func reservoir() []domain.Holding {
	return []domain.Holding{
		holding("r-cbtc", faucet, "CBTC", "10.0"),
		holding("r-usdcx", faucet, "USDCx", "1000000.0"),
	}
}

func Test_Faucet_TransfersThenQueuesAccepts(t *testing.T) {
	d := newDeps(t)
	u := d.startedWithFaucet(t, faucetConfig(t))

	d.redis.EXPECT().ClaimFaucet(Any, alice, time.Hour).Return(true, nil).Once()
	d.canton.EXPECT().Holdings(Any, faucet).Return(reservoir(), nil).Once()
	d.registry.EXPECT().TransferCommand(Any, dripSpec("CBTC", "1.0", "r-cbtc")).Return(command("t-cbtc"), nil).Once()
	d.registry.EXPECT().TransferCommand(Any, dripSpec("USDCx", "100000.0", "r-usdcx")).
		Return(command("t-usdcx"), nil).Once()
	d.canton.EXPECT().Submit(Any, faucet, faucetCommandID(), command("t-cbtc")).
		Return(domain.Transaction{Created: []string{"change-cbtc", "offer-cbtc"}}, nil).Once()
	d.canton.EXPECT().Submit(Any, faucet, faucetCommandID(), command("t-usdcx")).
		Return(domain.Transaction{Created: []string{"offer-usdcx"}}, nil).Once()
	d.canton.EXPECT().TransferInstructions(Any, alice).Return([]domain.IncomingTransfer{
		{ContractID: "offer-cbtc", Sender: faucet, Admin: operator, InstrumentID: "CBTC", Amount: decimal.MustParse("1")},
		{ContractID: "older", Sender: faucet, Admin: operator, InstrumentID: "CBTC", Amount: decimal.MustParse("1")},
		{ContractID: "offer-usdcx", Sender: faucet, Admin: operator, InstrumentID: "USDCx",
			Amount: decimal.MustParse("100000")},
	}, nil).Once()
	d.registry.EXPECT().AcceptTransferCommand(Any, registryURL, "offer-cbtc").Return(command("accept-cbtc"), nil).Once()
	d.registry.EXPECT().AcceptTransferCommand(Any, registryURL, "offer-usdcx").Return(command("accept-usdcx"), nil).Once()
	d.canton.EXPECT().Prepare(Any, alice, "accept:offer-cbtc", command("accept-cbtc")).
		Return(prepared("hash-cbtc"), nil).Once()
	d.canton.EXPECT().Prepare(Any, alice, "accept:offer-usdcx", command("accept-usdcx")).
		Return(prepared("hash-usdcx"), nil).Once()
	d.postgres.EXPECT().SaveActions(Any, mock.MatchedBy(func(actions []domain.Action) bool {
		return len(actions) == 2 &&
			actions[0].Ref == "offer-cbtc" && actions[0].Description == "Accept 1.0 CBTC from the faucet" &&
			actions[1].Ref == "offer-usdcx" && actions[1].Purpose == domain.PurposeTransferAccept
	})).Return(nil).Once()
	d.events.EXPECT().Publish(Any, eventOfType(events.SignRequested), parties(alice)).Return(nil).Times(2)

	output, err := u.Faucet(t.Context(), dto.FaucetInput{PartyID: alice})
	require.NoError(t, err)
	require.Len(t, output.Actions, 2)
	require.Equal(t, "hash-cbtc", string(output.Actions[0].Hash))

	network := u.GetNetwork(t.Context())
	require.Equal(t, domain.NetworkDevNet, network.Network)
	require.Equal(t, []dto.Drip{
		{Symbol: "CBTC", Amount: decimal.MustParse("1")},
		{Symbol: "USDCx", Amount: decimal.MustParse("100000")},
	}, network.Faucet)
}

func meeting(count int32) func() error {
	var arrived atomic.Int32

	all := make(chan struct{})

	return func() error {
		if arrived.Add(1) == count {
			close(all)
		}

		select {
		case <-all:
			return nil
		case <-time.After(time.Second):
			return errors.New("the calls ran one after another")
		}
	}
}

func (d deps) expectFaucetDrip(
	submit func(tx domain.Transaction) func(context.Context, string, string, domain.Command) (domain.Transaction, error),
	prepare func(hash string) func(context.Context, string, string, domain.Command) (domain.Prepared, error),
) {
	d.redis.EXPECT().ClaimFaucet(Any, alice, time.Hour).Return(true, nil).Once()
	d.canton.EXPECT().Holdings(Any, faucet).Return(reservoir(), nil).Once()
	d.registry.EXPECT().TransferCommand(Any, dripSpec("CBTC", "1.0", "r-cbtc")).Return(command("t-cbtc"), nil).Once()
	d.registry.EXPECT().TransferCommand(Any, dripSpec("USDCx", "100000.0", "r-usdcx")).
		Return(command("t-usdcx"), nil).Once()
	d.canton.EXPECT().Submit(Any, faucet, faucetCommandID(), command("t-cbtc")).
		RunAndReturn(submit(domain.Transaction{Created: []string{"offer-cbtc"}})).Once()
	d.canton.EXPECT().Submit(Any, faucet, faucetCommandID(), command("t-usdcx")).
		RunAndReturn(submit(domain.Transaction{Created: []string{"offer-usdcx"}})).Once()
	d.canton.EXPECT().TransferInstructions(Any, alice).Return([]domain.IncomingTransfer{
		{ContractID: "offer-cbtc", Sender: faucet, Admin: operator, InstrumentID: "CBTC", Amount: decimal.MustParse("1")},
		{ContractID: "offer-usdcx", Sender: faucet, Admin: operator, InstrumentID: "USDCx",
			Amount: decimal.MustParse("100000")},
	}, nil).Once()
	d.registry.EXPECT().AcceptTransferCommand(Any, registryURL, "offer-cbtc").Return(command("accept-cbtc"), nil).Once()
	d.registry.EXPECT().AcceptTransferCommand(Any, registryURL, "offer-usdcx").Return(command("accept-usdcx"), nil).Once()
	d.canton.EXPECT().Prepare(Any, alice, "accept:offer-cbtc", command("accept-cbtc")).
		RunAndReturn(prepare("hash-cbtc")).Once()
	d.canton.EXPECT().Prepare(Any, alice, "accept:offer-usdcx", command("accept-usdcx")).
		RunAndReturn(prepare("hash-usdcx")).Once()
	d.postgres.EXPECT().SaveActions(Any, Any).Return(nil).Maybe()
	d.events.EXPECT().Publish(Any, eventOfType(events.SignRequested), parties(alice)).Return(nil).Maybe()
}

func submitted(meet func() error) func(domain.Transaction) func(context.Context, string, string, domain.Command) (domain.Transaction, error) {
	return func(tx domain.Transaction) func(context.Context, string, string, domain.Command) (domain.Transaction, error) {
		return func(context.Context, string, string, domain.Command) (domain.Transaction, error) {
			err := meet()
			if err != nil {
				return domain.Transaction{}, err
			}

			return tx, nil
		}
	}
}

func preparedWhen(meet func() error) func(string) func(context.Context, string, string, domain.Command) (domain.Prepared, error) {
	return func(hash string) func(context.Context, string, string, domain.Command) (domain.Prepared, error) {
		return func(context.Context, string, string, domain.Command) (domain.Prepared, error) {
			err := meet()
			if err != nil {
				return domain.Prepared{}, err
			}

			return prepared(hash), nil
		}
	}
}

func Test_Faucet_DripsEveryInstrumentAtOnce(t *testing.T) {
	d := newDeps(t)
	u := d.startedWithFaucet(t, faucetConfig(t))

	d.expectFaucetDrip(submitted(meeting(2)), preparedWhen(func() error { return nil }))

	output, err := u.Faucet(t.Context(), dto.FaucetInput{PartyID: alice})
	require.NoError(t, err)
	require.Len(t, output.Actions, 2, "every drip lands while the others are still on their way")
}

func Test_Faucet_PreparesTheAcceptsAtOnce(t *testing.T) {
	d := newDeps(t)
	u := d.startedWithFaucet(t, faucetConfig(t))

	d.expectFaucetDrip(submitted(func() error { return nil }), preparedWhen(meeting(2)))

	output, err := u.Faucet(t.Context(), dto.FaucetInput{PartyID: alice})
	require.NoError(t, err)
	require.Equal(t, []string{"offer-cbtc", "offer-usdcx"}, []string{output.Actions[0].Ref, output.Actions[1].Ref},
		"the accepts keep the order of the incoming transfers")
}

func Test_Faucet_Cooldown(t *testing.T) {
	d := newDeps(t)
	u := d.startedWithFaucet(t, faucetConfig(t))

	d.redis.EXPECT().ClaimFaucet(Any, alice, time.Hour).Return(false, nil).Once()

	_, err := u.Faucet(t.Context(), dto.FaucetInput{PartyID: alice})
	require.ErrorIs(t, err, domain.ErrFaucetCooldown)
}

func Test_Faucet_DryReservoirReleasesTheClaim(t *testing.T) {
	d := newDeps(t)
	u := d.startedWithFaucet(t, faucetConfig(t))

	d.redis.EXPECT().ClaimFaucet(Any, alice, time.Hour).Return(true, nil).Once()
	d.canton.EXPECT().Holdings(Any, faucet).Return(nil, nil).Once()
	d.redis.EXPECT().ReleaseFaucet(Any, alice).Once()

	_, err := u.Faucet(t.Context(), dto.FaucetInput{PartyID: alice})
	require.ErrorIs(t, err, domain.ErrInsufficientFunds)
	require.ErrorContains(t, err, "the faucet sent nothing")
}

func Test_Faucet_CannotPayItself(t *testing.T) {
	d := newDeps(t)
	u := d.startedWithFaucet(t, faucetConfig(t))

	_, err := u.Faucet(t.Context(), dto.FaucetInput{PartyID: faucet})
	require.ErrorIs(t, err, domain.ErrValidation)
}

func Test_Faucet_OffUnlessDevNet(t *testing.T) {
	c := faucetConfig(t)
	c.Network = domain.NetworkMainNet

	d := newDeps(t)
	u := d.startedWithFaucet(t, c)

	_, err := u.Faucet(t.Context(), dto.FaucetInput{PartyID: alice})
	require.ErrorIs(t, err, domain.ErrFaucetUnavailable)

	network := u.GetNetwork(t.Context())
	require.Equal(t, domain.NetworkMainNet, network.Network)
	require.Empty(t, network.Faucet)
}

func Test_Faucet_OffWithoutAParty(t *testing.T) {
	d := newDeps(t)
	u := d.started(t)

	_, err := u.Faucet(t.Context(), dto.FaucetInput{PartyID: alice})
	require.ErrorIs(t, err, domain.ErrFaucetUnavailable)
}
