package canton

import (
	"bufio"
	"crypto/sha1" //nolint:gosec
	"encoding/base64"
	"encoding/binary"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/domain"
)

const (
	opText  = 0x1
	opClose = 0x8
)

type socket struct {
	rw   *bufio.ReadWriter
	stop <-chan struct{}
}

func (s socket) frame(t *testing.T, opcode byte, payload []byte) {
	t.Helper()

	header := []byte{0x80 | opcode}

	switch n := len(payload); {
	case n < 126:
		header = append(header, byte(n))
	case n < 1<<16:
		header = binary.BigEndian.AppendUint16(append(header, 126), uint16(n))
	default:
		header = binary.BigEndian.AppendUint64(append(header, 127), uint64(n))
	}

	_, err := s.rw.Write(append(header, payload...))
	assert.NoError(t, err)
	assert.NoError(t, s.rw.Flush())
}

func (s socket) text(t *testing.T, payload string) {
	t.Helper()
	s.frame(t, opText, []byte(payload))
}

func (s socket) closeWith(t *testing.T, codes ...uint16) {
	t.Helper()

	var payload []byte
	for _, code := range codes {
		payload = binary.BigEndian.AppendUint16(payload, code)
	}

	s.frame(t, opClose, payload)
}

func (s socket) readClientFrame(t *testing.T) []byte {
	t.Helper()

	var head [2]byte
	_, err := io.ReadFull(s.rw, head[:])
	require.NoError(t, err)

	n := uint64(head[1] & 0x7f)

	switch n {
	case 126:
		var ext [2]byte
		_, err = io.ReadFull(s.rw, ext[:])
		n = uint64(binary.BigEndian.Uint16(ext[:]))
	case 127:
		var ext [8]byte
		_, err = io.ReadFull(s.rw, ext[:])
		n = binary.BigEndian.Uint64(ext[:])
	}

	require.NoError(t, err)

	var mask [4]byte
	_, err = io.ReadFull(s.rw, mask[:])
	require.NoError(t, err)

	payload := make([]byte, n)
	_, err = io.ReadFull(s.rw, payload)
	require.NoError(t, err)

	for i := range payload {
		payload[i] ^= mask[i%4]
	}

	return payload
}

func streamServer(t *testing.T, script func(s socket)) *Canton {
	t.Helper()

	otel.SilentModeInit()

	stop := make(chan struct{})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/state/ledger-end" {
			_, _ = io.WriteString(w, `{"offset":7}`)

			return
		}

		accept := sha1.Sum([]byte(r.Header.Get("Sec-WebSocket-Key") + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11")) //nolint:gosec

		conn, rw, err := http.NewResponseController(w).Hijack()
		if !assert.NoError(t, err) {
			return
		}

		defer func() { _ = conn.Close() }()

		_, _ = rw.WriteString("HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n" +
			"Sec-WebSocket-Accept: " + base64.StdEncoding.EncodeToString(accept[:]) + "\r\n" +
			"Sec-WebSocket-Protocol: daml.ws.auth\r\n\r\n")
		_ = rw.Flush()

		s := socket{rw: rw, stop: stop}
		assert.Contains(t, string(s.readClientFrame(t)), `"activeAtOffset":7`)

		script(s)
	}))

	t.Cleanup(func() {
		close(stop)
		srv.Close()
	})

	c := New(Config{URL: srv.URL})
	c.idleTimeout = 300 * time.Millisecond

	return c
}

func Test_ActiveContracts_CloseHandling(t *testing.T) {
	for name, tc := range map[string]struct {
		script func(t *testing.T, s socket)
		ok     bool
	}{
		"normal closure (1000)": {func(t *testing.T, s socket) {
			s.text(t, holdingFrame)
			s.closeWith(t, 1000)
		}, true},
		"close without a status (1005)": {func(t *testing.T, s socket) {
			s.text(t, holdingFrame)
			s.closeWith(t)
		}, true},
		"going away (1001) may have cut the snapshot": {func(t *testing.T, s socket) {
			s.text(t, holdingFrame)
			s.closeWith(t, 1001)
		}, false},
		"dropped connection": {func(t *testing.T, s socket) {
			s.text(t, holdingFrame)
		}, false},
		"idle stream": {func(t *testing.T, s socket) {
			s.text(t, holdingFrame)
			<-s.stop
		}, false},
	} {
		t.Run(name, func(t *testing.T) {
			c := streamServer(t, func(s socket) { tc.script(t, s) })

			contracts, err := c.activeContracts(t.Context(), interfaceFormat("makerA::1220", holdingV1))
			if tc.ok {
				require.NoError(t, err)
				require.Len(t, contracts, 1)
				require.Equal(t, "00a1f5", contracts[0].CreatedEvent.ContractID)

				return
			}

			require.ErrorIs(t, err, domain.ErrLedger)
			require.ErrorIs(t, err, domain.ErrLedgerUnavailable, "a cut stream is worth retrying")
			require.Nil(t, contracts)
		})
	}
}

func Test_ActiveContracts_IdleTimeoutResetsOnEveryFrame(t *testing.T) {
	c := streamServer(t, func(s socket) {
		for range 8 {
			time.Sleep(60 * time.Millisecond)
			s.text(t, holdingFrame)
		}

		s.closeWith(t, 1000)
	})

	contracts, err := c.activeContracts(t.Context(), interfaceFormat("makerA::1220", holdingV1))
	require.NoError(t, err)
	require.Len(t, contracts, 8)
}

func Test_ActiveContracts_ErrorFrame(t *testing.T) {
	c := streamServer(t, func(s socket) {
		s.text(t, errorFrame)
		s.closeWith(t, 1000)
	})

	_, err := c.activeContracts(t.Context(), interfaceFormat("makerA::1220", holdingV1))
	require.ErrorIs(t, err, domain.ErrLedger)
	require.ErrorContains(t, err, "OFFSET_AFTER_LEDGER_END")
}
