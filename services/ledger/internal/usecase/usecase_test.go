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
	makerA   = "makerA::1220aa"
	makerB   = "makerB::1220aa"
	alice    = "alice-3f9a1c::1220bb" // an external party: it signs its own transactions
	syncID   = "synchronizer-1::1220cc"
)

// noInputs: a mint offer locks no holding of a sender, so its accept discloses none.
//
//nolint:gochecknoglobals
var noInputs []domain.Disclosed

//nolint:gochecknoglobals
var rules = domain.Disclosed{
	TemplateID:       "pkg:Splice.Testing.Tokens.TestTokenV2:TokenRules",
	ContractID:       "rules-cid",
	CreatedEventBlob: "rules-blob",
	SynchronizerID:   syncID,
}

func config(t *testing.T) usecase.Config {
	t.Helper()

	var instruments domain.InstrumentList
	require.NoError(t, instruments.UnmarshalText([]byte("CBTC:Bitcoin:8,USDCx:USD Coin:2,CETH:Ether:6")))

	return usecase.Config{
		OperatorHint: "operator",
		HouseParties: []string{"makerA", "makerB"},
		Instruments:  instruments,
		FaucetAmounts: domain.Amounts{
			"CBTC":  decimal.MustParse("1"),
			"USDCx": decimal.MustParse("100000"),
		},
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
		events:   mocks.NewEvents(t),
	}
}

func (d deps) useCase(t *testing.T) *usecase.UseCase {
	t.Helper()

	return usecase.New(config(t), d.tx, d.postgres, d.redis, d.canton, d.events)
}

// expectWarmUp is what Init does once the parties are known: it refreshes the caches, so
// a ledger restarted with new contract ids cannot serve stale ones.
func (d deps) expectWarmUp() {
	d.canton.EXPECT().TokenRules(Any, operator).Return(rules, nil).Once()
	d.redis.EXPECT().SetTokenRules(Any, rules).Once()
	d.canton.EXPECT().SynchronizerID(Any, operator).Return(syncID, nil).Once()
	d.redis.EXPECT().SetSynchronizerID(Any, syncID).Once()
}

// started is a use case after Init against a ledger that is up and seeded.
func (d deps) started(t *testing.T) *usecase.UseCase {
	t.Helper()

	d.canton.EXPECT().Ready(Any).Return(true).Once()
	d.canton.EXPECT().ResolveParty(Any, "operator").Return(operator, nil).Once()
	d.canton.EXPECT().ResolveParty(Any, "makerA").Return(makerA, nil).Once()
	d.canton.EXPECT().ResolveParty(Any, "makerB").Return(makerB, nil).Once()
	d.expectWarmUp()

	u := d.useCase(t)
	require.NoError(t, u.Init(t.Context()))

	return u
}

func (d deps) cachedRules() {
	d.redis.EXPECT().GetTokenRules(Any).Return(rules, true)
}

// event matches a published event by its JSON form, e.g. `{"actionId":"…"}` for data.
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

// tradeLegs: the taker buys 1 CBTC from the maker at 100000 USDCx, the maker pays a 100 USDCx fee.
func tradeLegs(maker, taker string) []dto.TransferLeg {
	return []dto.TransferLeg{
		{ID: "base", Sender: maker, Receiver: taker, Instrument: "CBTC", Amount: "1.0"},
		{ID: "quote", Sender: taker, Receiver: maker, Instrument: "USDCx", Amount: "100000.0"},
		{ID: "fee", Sender: maker, Receiver: operator, Instrument: "USDCx", Amount: "100.0"},
	}
}

func command(name string) domain.Command {
	return domain.Command{Payload: json.RawMessage(`{"name":"` + name + `"}`), Disclosed: []domain.Disclosed{rules}}
}

func prepared(hash string) domain.Prepared {
	return domain.Prepared{Transaction: "tx-" + hash, HashingSchemeVersion: "HASHING_SCHEME_VERSION_V2", Hash: []byte(hash)}
}
