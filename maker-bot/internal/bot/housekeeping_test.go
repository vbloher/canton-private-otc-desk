package bot_test

import (
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/maker-bot/internal/desk"
)

func faucetNetwork(t *testing.T) desk.Network {
	t.Helper()

	return desk.Network{Name: "devnet", Faucet: []desk.Drip{{Symbol: "CBTC", Amount: rat(t, "0.01")}}}
}

func Test_Housekeeping_AcceptsIncomingTransfers(t *testing.T) {
	d := &fakeDesk{transfers: []desk.Transfer{
		{ContractID: "offer-1", Sender: "faucet", Symbol: "CBTC", Amount: rat(t, "0.01")},
		{ContractID: "offer-2", Sender: "alice", Symbol: "USDCx", Amount: rat(t, "1000")},
	}}
	b, _, _ := newBot(t, d)

	b.Housekeeping(t.Context())

	require.Equal(t, []string{"offer-1", "offer-2"}, d.accepted)

	calls := d.callNames()
	require.Contains(t, calls[slicesIndex(calls, "AcceptTransfer"):], "PendingActions", "the accepts are signed right away")
}

func Test_Housekeeping_DrawsFromTheFaucetWhenLow(t *testing.T) {
	d := &fakeDesk{network: faucetNetwork(t), balances: map[string]*big.Rat{"CBTC": rat(t, "0.05")}}
	b, _, _ := newBot(t, d)

	b.Housekeeping(t.Context())

	require.Equal(t, 1, d.faucetCalls, "0.05 CBTC is under ten drips of 0.01")
}

func Test_Housekeeping_LeavesTheFaucetAloneAfterADrip(t *testing.T) {
	d := &fakeDesk{network: faucetNetwork(t), balances: map[string]*big.Rat{"CBTC": rat(t, "0.05")}}
	clock := noon
	b := botAt(t, d, &clock)

	b.Housekeeping(t.Context())
	b.Housekeeping(t.Context())
	require.Equal(t, 1, d.faucetCalls, "a drip that left a balance low is not asked for again at once")

	clock = noon.Add(61 * time.Minute)
	b.Housekeeping(t.Context())
	require.Equal(t, 2, d.faucetCalls, "an hour later the bot may draw again")
}

func Test_Housekeeping_LeavesTheFaucetAloneAfterACooldownAnswer(t *testing.T) {
	d := &fakeDesk{
		network:   faucetNetwork(t),
		balances:  map[string]*big.Rat{},
		faucetErr: fmt.Errorf("faucet: %w", desk.ErrTooManyRequests),
	}
	clock := noon
	b := botAt(t, d, &clock)

	for range 6 {
		b.Housekeeping(t.Context())
		clock = clock.Add(10 * time.Second)
	}

	require.Equal(t, 1, d.faucetCalls)
}

func Test_Housekeeping_SkipsTheFaucetWhenFunded(t *testing.T) {
	d := &fakeDesk{network: faucetNetwork(t), balances: map[string]*big.Rat{"CBTC": rat(t, "0.2")}}
	b, _, _ := newBot(t, d)

	b.Housekeeping(t.Context())

	require.Zero(t, d.faucetCalls)
}

func Test_Housekeeping_FaucetCooldownIsQuiet(t *testing.T) {
	d := &fakeDesk{
		network:   faucetNetwork(t),
		balances:  map[string]*big.Rat{},
		faucetErr: fmt.Errorf("faucet: %w", desk.ErrTooManyRequests),
	}
	b, _, out := newBot(t, d)

	b.Housekeeping(t.Context())

	require.Equal(t, 1, d.faucetCalls)
	require.Zero(t, out.count(t, "WARN", ""))
	require.Zero(t, out.count(t, "ERROR", ""))
}

func Test_Housekeeping_NoFaucetOnTheNetwork(t *testing.T) {
	d := &fakeDesk{network: desk.Network{Name: "mainnet"}}
	b, _, _ := newBot(t, d)

	b.Housekeeping(t.Context())

	require.NotContains(t, d.callNames(), "Balances")
	require.Zero(t, d.faucetCalls)
}

func slicesIndex(calls []string, name string) int {
	for i, call := range calls {
		if call == name {
			return i
		}
	}

	return len(calls)
}
