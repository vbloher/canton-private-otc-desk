package usecase_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/pkg/decimal"
	"github.com/vbloher/canton-private-otc-desk/pkg/events"
	"github.com/vbloher/canton-private-otc-desk/pkg/otel"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/domain"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/dto"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/usecase"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/usecase/mocks"
)

const (
	Any = mock.Anything

	operator = "operator::1220aa"
	maker    = "maker::1220aa"
	bob      = "bob::1220ee"
	alice    = "alice-3f9a1c::1220bb"
	faucet   = "faucet::1220aa"
	dso      = "DSO::1220dd"
	syncID   = "synchronizer-1::1220cc"

	registryURL = "https://registry.test/operator/registry"
	scanURL     = "https://scan.test/registry"
)

func config(t *testing.T) usecase.Config {
	t.Helper()

	var instruments domain.InstrumentList
	require.NoError(t, instruments.UnmarshalText([]byte(
		"CBTC|Bitcoin|8|CBTC|"+operator+"|"+registryURL+","+
			"USDCx|USD Coin|2|USDCx|"+operator+"|"+registryURL+","+
			"CC|Canton Coin|4|Amulet|"+dso+"|"+scanURL)))

	return usecase.Config{
		Network:       domain.NetworkDevNet,
		OperatorParty: operator,
		MakerParty:    maker,
		Instruments:   instruments,
		FaucetAmounts: domain.Amounts{
			"CBTC":  decimal.MustParse("1"),
			"USDCx": decimal.MustParse("100000"),
		},
		FaucetCooldown:  time.Hour,
		ActionTTL:       10 * time.Minute,
		StartupTimeout:  5 * time.Second,
		StartupInterval: time.Millisecond,
	}
}

type deps struct {
	tx       *mocks.Transactor
	postgres *mocks.Postgres
	redis    *mocks.Redis
	canton   *mocks.Canton
	registry *mocks.Registry
	events   *mocks.Events
}

func newDeps(t *testing.T) deps {
	t.Helper()

	otel.SilentModeInit()

	tx := mocks.NewTransactor(t)
	tx.EXPECT().Wrap(Any, Any).RunAndReturn(func(ctx context.Context, fn func(context.Context) error) error {
		return fn(ctx)
	}).Maybe()

	return deps{
		tx:       tx,
		postgres: mocks.NewPostgres(t),
		redis:    mocks.NewRedis(t),
		canton:   mocks.NewCanton(t),
		registry: mocks.NewRegistry(t),
		events:   mocks.NewEvents(t),
	}
}

func (d deps) useCase(t *testing.T) *usecase.UseCase {
	t.Helper()

	return usecase.New(config(t), d.tx, d.postgres, d.redis, d.canton, d.registry, d.events)
}

func (d deps) expectWarmUp() {
	d.canton.EXPECT().SynchronizerID(Any, operator).Return(syncID, nil).Once()
	d.redis.EXPECT().SetSynchronizerID(Any, syncID).Once()
}

func (d deps) started(t *testing.T) *usecase.UseCase {
	t.Helper()

	d.canton.EXPECT().Ready(Any).Return(true).Once()
	d.canton.EXPECT().ResolveParty(Any, operator).Return(operator, nil).Once()
	d.canton.EXPECT().ResolveParty(Any, maker).Return(maker, nil).Once()
	d.expectWarmUp()

	u := d.useCase(t)
	require.NoError(t, u.Init(t.Context()))

	return u
}

func event(typ, data string) any {
	want := fmt.Sprintf(`{"type":%q,"data":%s}`, typ, data)

	return mock.MatchedBy(func(ev events.Event) bool {
		got, err := json.Marshal(ev)

		return err == nil && string(got) == want
	})
}

func eventOfType(typ string) any {
	return mock.MatchedBy(func(ev events.Event) bool { return ev.Type == typ })
}

func parties(ids ...string) []string {
	return ids
}

func holding(cid, owner, instrument, amount string) domain.Holding {
	return domain.Holding{
		ContractID:   cid,
		Owner:        owner,
		Admin:        operator,
		InstrumentID: instrument,
		Amount:       decimal.MustParse(amount),
	}
}

func tradeLegs(maker, taker string) []dto.TransferLeg {
	return []dto.TransferLeg{
		{ID: "base", Sender: maker, Receiver: taker, Instrument: "CBTC", Amount: "1.0"},
		{ID: "quote", Sender: taker, Receiver: maker, Instrument: "USDCx", Amount: "100000.0"},
		{ID: "fee", Sender: maker, Receiver: operator, Instrument: "USDCx", Amount: "100.0"},
	}
}

func command(name string) domain.Command {
	return domain.Command{Commands: []json.RawMessage{json.RawMessage(`{"name":"` + name + `"}`)}}
}

func prepared(hash string) domain.Prepared {
	return domain.Prepared{Transaction: "tx-" + hash, HashingSchemeVersion: "HASHING_SCHEME_VERSION_V2", Hash: []byte(hash)}
}
