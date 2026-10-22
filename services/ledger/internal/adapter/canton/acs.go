package canton

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/coder/websocket"

	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/domain"
)

// One contract per frame; a frame carrying created-event blobs of large contracts can pass
// the library's 32 KiB default.
const maxFrame = 16 << 20

type activeContract struct {
	CreatedEvent   createdEvent `json:"createdEvent"`
	SynchronizerID string       `json:"synchronizerId"`
}

type createdEvent struct {
	ContractID       string          `json:"contractId"`
	TemplateID       string          `json:"templateId"`
	CreatedEventBlob string          `json:"createdEventBlob"`
	InterfaceViews   []interfaceView `json:"interfaceViews"`
}

type interfaceView struct {
	InterfaceID string          `json:"interfaceId"`
	ViewValue   json.RawMessage `json:"viewValue"`
}

// views are the computed interface views of the contract (a failed view has none).
func (e createdEvent) views() []json.RawMessage {
	var out []json.RawMessage

	for _, v := range e.InterfaceViews {
		if len(v.ViewValue) > 0 && string(v.ViewValue) != "null" {
			out = append(out, v.ViewValue)
		}
	}

	return out
}

type eventFormat struct {
	FiltersByParty map[string]filters `json:"filtersByParty"`
	Verbose        bool               `json:"verbose"`
}

type filters struct {
	Cumulative []cumulativeFilter `json:"cumulative"`
}

type cumulativeFilter struct {
	IdentifierFilter identifierFilter `json:"identifierFilter"`
}

type identifierFilter struct {
	WildcardFilter  *wildcardFilter  `json:"WildcardFilter,omitempty"`
	TemplateFilter  *templateFilter  `json:"TemplateFilter,omitempty"`
	InterfaceFilter *interfaceFilter `json:"InterfaceFilter,omitempty"`
}

type wildcardFilter struct {
	Value struct {
		IncludeCreatedEventBlob bool `json:"includeCreatedEventBlob"`
	} `json:"value"`
}

type templateFilter struct {
	Value struct {
		TemplateID              string `json:"templateId"`
		IncludeCreatedEventBlob bool   `json:"includeCreatedEventBlob"`
	} `json:"value"`
}

type interfaceFilter struct {
	Value struct {
		InterfaceID             string `json:"interfaceId"`
		IncludeInterfaceView    bool   `json:"includeInterfaceView"`
		IncludeCreatedEventBlob bool   `json:"includeCreatedEventBlob"`
	} `json:"value"`
}

func partyFilter(party string, f identifierFilter) eventFormat {
	return eventFormat{FiltersByParty: map[string]filters{
		party: {Cumulative: []cumulativeFilter{{IdentifierFilter: f}}},
	}}
}

// wildcardFormat shows everything the parties see (the events of a submitted transaction).
func wildcardFormat(party string) eventFormat {
	return partyFilter(party, identifierFilter{WildcardFilter: &wildcardFilter{}})
}

// templateFormat narrows a read to one template server-side, with the created-event blobs
// a disclosure needs.
func templateFormat(party, templateID string) eventFormat {
	f := &templateFilter{}
	f.Value.TemplateID = templateID
	f.Value.IncludeCreatedEventBlob = true

	return partyFilter(party, identifierFilter{TemplateFilter: f})
}

// interfaceFormat reads every contract implementing the interface, with its view.
func interfaceFormat(party, interfaceID string) eventFormat {
	f := &interfaceFilter{}
	f.Value.InterfaceID = interfaceID
	f.Value.IncludeInterfaceView = true

	return partyFilter(party, identifierFilter{InterfaceFilter: f})
}

// activeContracts reads the active contracts matching format at the current ledger end
// over the WebSocket stream, which is not bounded by the HTTP list cap. A large stream is
// fine as long as frames keep coming: only a silence longer than the idle timeout aborts
// it. It succeeds only on a clean close: a truncated stream is an error, never a partial
// ACS.
func (c *Canton) activeContracts(ctx context.Context, format eventFormat) ([]activeContract, error) {
	var end struct {
		Offset int64 `json:"offset"`
	}

	err := c.call(ctx, http.MethodGet, "/v2/state/ledger-end", nil, &end, readTimeout)
	if err != nil {
		return nil, fmt.Errorf("ledger-end: %w", err)
	}

	conn, err := c.dialActiveContracts(ctx, end.Offset, format)
	if err != nil {
		return nil, err
	}

	defer func() { _ = conn.CloseNow() }()

	return readContracts(ctx, conn, c.idleTimeout)
}

// dialActiveContracts opens the stream and sends the request, within readTimeout.
func (c *Canton) dialActiveContracts(ctx context.Context, offset int64, format eventFormat) (*websocket.Conn, error) {
	ctx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()

	conn, resp, err := websocket.Dial(ctx, c.wsURL+"/v2/state/active-contracts", &websocket.DialOptions{
		Subprotocols: []string{"daml.ws.auth"},
	})
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}

	if err != nil {
		return nil, unavailable(fmt.Errorf("active-contracts: %w", err))
	}

	conn.SetReadLimit(maxFrame)

	request, err := json.Marshal(struct {
		ActiveAtOffset int64       `json:"activeAtOffset"`
		EventFormat    eventFormat `json:"eventFormat"`
	}{ActiveAtOffset: offset, EventFormat: format})
	if err != nil {
		_ = conn.CloseNow()

		return nil, fmt.Errorf("json.Marshal: %w", err)
	}

	err = conn.Write(ctx, websocket.MessageText, request)
	if err != nil {
		_ = conn.CloseNow()

		return nil, unavailable(fmt.Errorf("active-contracts: %w", err))
	}

	return conn, nil
}

// readContracts reads frames until the server closes the stream; every frame resets the
// idle timeout.
func readContracts(ctx context.Context, conn *websocket.Conn, idle time.Duration) ([]activeContract, error) {
	var contracts []activeContract

	for {
		frame, err := readFrame(ctx, conn, idle)
		if err != nil {
			// 1005: the close frame carried no status after a complete stream. Anything else
			// (1001 going away, a dropped connection, an idle stream) may have cut it short.
			status := websocket.CloseStatus(err)
			if status == websocket.StatusNormalClosure || status == websocket.StatusNoStatusRcvd {
				return contracts, nil
			}

			return nil, unavailable(fmt.Errorf("active-contracts stream: %w", err))
		}

		contract, err := parseACSFrame(frame)
		if err != nil {
			return nil, err
		}

		if contract != nil {
			contracts = append(contracts, *contract)
		}
	}
}

func readFrame(ctx context.Context, conn *websocket.Conn, idle time.Duration) ([]byte, error) {
	frameCtx, cancel := context.WithTimeout(ctx, idle)
	defer cancel()

	_, frame, err := conn.Read(frameCtx)
	if err != nil && ctx.Err() == nil && errors.Is(frameCtx.Err(), context.DeadlineExceeded) {
		return nil, fmt.Errorf("no frame for %s: %w", idle, err)
	}

	return frame, err //nolint:wrapcheck // the caller wraps it once it has read the close status
}

// parseACSFrame returns the frame's active contract, nil for a frame without one (offset
// markers, other entry kinds), or the error an error frame carries.
func parseACSFrame(data []byte) (*activeContract, error) {
	var frame struct {
		ContractEntry *struct {
			JsActiveContract *activeContract `json:"JsActiveContract"`
		} `json:"contractEntry"`
		Code          string          `json:"code"`
		Cause         json.RawMessage `json:"cause"`
		ErrorCategory int             `json:"errorCategory"`
	}

	err := json.Unmarshal(data, &frame)
	if err != nil {
		return nil, fmt.Errorf("%w: active-contracts frame: %w", domain.ErrLedger, err)
	}

	if frame.ContractEntry != nil && frame.ContractEntry.JsActiveContract != nil {
		return frame.ContractEntry.JsActiveContract, nil
	}

	if frame.Code != "" {
		var cause string
		if json.Unmarshal(frame.Cause, &cause) != nil {
			cause = string(frame.Cause)
		}

		return nil, ledgerError(frame.Code, cause, frame.ErrorCategory, 0)
	}

	return nil, nil //nolint:nilnil // a frame without a contract is skipped
}
