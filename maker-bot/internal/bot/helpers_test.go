package bot_test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"log/slog"
	"math/big"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/maker-bot/internal/bot"
	"github.com/vbloher/canton-private-otc-desk/maker-bot/internal/config"
	"github.com/vbloher/canton-private-otc-desk/maker-bot/internal/desk"
	"github.com/vbloher/canton-private-otc-desk/maker-bot/internal/identity"
	"github.com/vbloher/canton-private-otc-desk/maker-bot/internal/stream"
)

var noon = time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)

type logs struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (l *logs) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	return l.buf.Write(p)
}

func (l *logs) records(t *testing.T) []map[string]any {
	t.Helper()

	l.mu.Lock()
	defer l.mu.Unlock()

	var out []map[string]any

	scanner := bufio.NewScanner(bytes.NewReader(l.buf.Bytes()))
	for scanner.Scan() {
		var record map[string]any
		require.NoError(t, json.Unmarshal(scanner.Bytes(), &record))
		out = append(out, record)
	}

	return out
}

func (l *logs) count(t *testing.T, level, msg string) int {
	t.Helper()

	n := 0

	for _, r := range l.records(t) {
		if r["level"] == level && strings.Contains(r["msg"].(string), msg) {
			n++
		}
	}

	return n
}

func newBot(t *testing.T, d *fakeDesk) (*bot.Bot, identity.Identity, *logs) {
	t.Helper()

	c, err := config.Parse(nil)
	require.NoError(t, err)

	id, err := identity.Generate()
	require.NoError(t, err)

	out := &logs{}
	log := slog.New(slog.NewJSONHandler(out, &slog.HandlerOptions{Level: slog.LevelDebug}))

	return bot.New(d, id, c, log, func() time.Time { return noon }), id, out
}

func botAt(t *testing.T, d *fakeDesk, clock *time.Time) *bot.Bot {
	t.Helper()

	c, err := config.Parse(nil)
	require.NoError(t, err)

	id, err := identity.Generate()
	require.NoError(t, err)

	return bot.New(d, id, c, slog.New(slog.NewJSONHandler(&logs{}, nil)), func() time.Time { return *clock })
}

func rat(t *testing.T, s string) *big.Rat {
	t.Helper()

	r, ok := new(big.Rat).SetString(s)
	require.True(t, ok, s)

	return r
}

func funded(t *testing.T) map[string]*big.Rat {
	t.Helper()

	return map[string]*big.Rat{"CBTC": rat(t, "10"), "USDCx": rat(t, "10000000"), "CC": rat(t, "10000000")}
}

func openRFQ(t *testing.T, base, quote string, takerBuys bool, qty string, closesIn time.Duration) desk.RFQ {
	t.Helper()

	return desk.RFQ{
		ID: uuid.New(), TakerBuys: takerBuys, Qty: rat(t, qty), Base: base, Quote: quote, FeeBps: 10,
		Deadline: noon.Add(closesIn), Open: true,
	}
}

func quoted(status string) *desk.Quote {
	return &desk.Quote{ID: uuid.New(), Status: status}
}

func event(t *testing.T, typ string, data any) stream.Event {
	t.Helper()

	raw, err := json.Marshal(data)
	require.NoError(t, err)

	return stream.Event{Type: typ, Data: raw}
}

func submitted(d *fakeDesk) []uuid.UUID {
	d.mu.Lock()
	defer d.mu.Unlock()

	ids := make([]uuid.UUID, 0, len(d.submissions))
	for _, s := range d.submissions {
		ids = append(ids, s.RFQID)
	}

	return ids
}
