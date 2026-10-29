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
	InterfaceFilter *interfaceFilter `json:"InterfaceFilter,omitempty"`
}

type wildcardFilter struct {
	Value struct {
		IncludeCreatedEventBlob bool `json:"includeCreatedEventBlob"`
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

func wildcardFormat(party string) eventFormat {
	return partyFilter(party, identifierFilter{WildcardFilter: &wildcardFilter{}})
}

func interfaceFormat(party, interfaceID string) eventFormat {
	f := &interfaceFilter{}
	f.Value.InterfaceID = interfaceID
	f.Value.IncludeInterfaceView = true

	return partyFilter(party, identifierFilter{InterfaceFilter: f})
}

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

func (c *Canton) dialActiveContracts(ctx context.Context, offset int64, format eventFormat) (*websocket.Conn, error) {
	ctx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()

	protocols := []string{"daml.ws.auth"}

	token, err := c.token()
	if err != nil {
		return nil, err
	}

	if token != "" {
		protocols = append([]string{"jwt.token." + token}, protocols...)
	}

	conn, resp, err := websocket.Dial(ctx, c.wsURL+"/v2/state/active-contracts", &websocket.DialOptions{
		Subprotocols: protocols,
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

func readContracts(ctx context.Context, conn *websocket.Conn, idle time.Duration) ([]activeContract, error) {
	var contracts []activeContract

	for {
		frame, err := readFrame(ctx, conn, idle)
		if err != nil {
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

	return frame, err //nolint:wrapcheck
}

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

	return nil, nil //nolint:nilnil
}
