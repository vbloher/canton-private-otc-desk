package bot_test

import (
	"errors"
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/maker-bot/internal/desk"
)

func faucetNetwork(t *testing.T) desk.Network {
	t.Helper()

	return desk.Network{Name: "devnet", Faucet: []desk.Drip{
		{Symbol: "CBTC", Amount: rat(t, "0.01")},
		{Symbol: "CC", Amount: rat(t, "5000")},
	}}
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

func Test_Housekeeping_DrawsFromTheFaucetWhenABalanceIsUnderADrip(t *testing.T) {
	d := &fakeDesk{network: faucetNetwork(t), balances: map[string]*big.Rat{"CBTC": rat(t, "0.05"), "CC": rat(t, "4999")}}
	b, _, out := newBot(t, d)

	b.Housekeeping(t.Context())

	require.Equal(t, 1, d.faucetCalls, "4999 CC is under one drip of 5000")
	require.Equal(t, 1, out.count(t, "INFO", "faucet drip requested"))
}

func Test_Housekeeping_SkipsTheFaucetWhileEveryBalanceCoversADrip(t *testing.T) {
	d := &fakeDesk{network: faucetNetwork(t), balances: map[string]*big.Rat{"CBTC": rat(t, "0.05"), "CC": rat(t, "14933")}}
	b, _, _ := newBot(t, d)

	b.Housekeeping(t.Context())

	require.Zero(t, d.faucetCalls, "the faucet is shared, and a drip of each is enough to quote")
}

func Test_Housekeeping_DrawsAgainOnceADripHasArrived(t *testing.T) {
	d := &fakeDesk{
		network:     faucetNetwork(t),
		balances:    map[string]*big.Rat{"CBTC": rat(t, "0.005"), "CC": rat(t, "5000")},
		faucetSends: map[string]*big.Rat{"CBTC": rat(t, "0.01"), "CC": rat(t, "5000")},
	}
	clock := noon
	b, _ := botAt(t, d, &clock)

	b.Housekeeping(t.Context())
	clock = clock.Add(10 * time.Second)
	b.Housekeeping(t.Context())
	require.Equal(t, 1, d.faucetCalls, "a drip is not asked for again at once")

	d.balances["CBTC"] = rat(t, "0.001")
	clock = noon.Add(61 * time.Minute)
	b.Housekeeping(t.Context())
	require.Equal(t, 2, d.faucetCalls, "the drip arrived and was spent, so an hour later the bot draws again")
}

func Test_Housekeeping_StopsDrawingWhenADripDoesNotArrive(t *testing.T) {
	d := &fakeDesk{
		network:     faucetNetwork(t),
		balances:    map[string]*big.Rat{"CBTC": rat(t, "0.05"), "CC": rat(t, "4000")},
		faucetSends: map[string]*big.Rat{"CBTC": rat(t, "0.01")},
	}
	clock := noon
	b, out := botAt(t, d, &clock)

	for range 4 {
		b.Housekeeping(t.Context())
		clock = clock.Add(61 * time.Minute)
	}

	require.Equal(t, 1, d.faucetCalls, "the faucet sent CBTC but no CC, so it is not asked again")
	require.Equal(t, 1, out.count(t, "WARN", "the faucet did not refill"))
}

func Test_Housekeeping_StopsDrawingAfterAFailedDrip(t *testing.T) {
	d := &fakeDesk{network: faucetNetwork(t), balances: map[string]*big.Rat{}, faucetErr: errors.New("the faucet sent nothing")}
	clock := noon
	b, out := botAt(t, d, &clock)

	for range 3 {
		b.Housekeeping(t.Context())
		clock = clock.Add(61 * time.Minute)
	}

	require.Equal(t, 1, d.faucetCalls)
	require.Equal(t, 1, out.count(t, "WARN", "the faucet did not refill"))
}

func Test_Housekeeping_AsksAgainAfterACooldownAnswer(t *testing.T) {
	d := &fakeDesk{
		network:   faucetNetwork(t),
		balances:  map[string]*big.Rat{},
		faucetErr: fmt.Errorf("faucet: %w", desk.ErrTooManyRequests),
	}
	clock := noon
	b, _ := botAt(t, d, &clock)

	for range 6 {
		b.Housekeeping(t.Context())
		clock = clock.Add(10 * time.Second)
	}

	require.Equal(t, 1, d.faucetCalls, "a cooling faucet is left alone for an hour")

	clock = noon.Add(61 * time.Minute)
	b.Housekeeping(t.Context())
	require.Equal(t, 2, d.faucetCalls, "a cooldown answer is not a missing drip")
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
