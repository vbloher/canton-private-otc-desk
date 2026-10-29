package canton

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/domain"
)

func jsonAPI(t *testing.T, path string, status int, body string) (*Canton, *[]byte) {
	t.Helper()

	otel.SilentModeInit()

	var got []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != path {
			http.NotFound(w, r)

			return
		}

		got, _ = io.ReadAll(r.Body)

		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)

	return New(Config{URL: srv.URL, UserID: "participant_admin"}), &got
}

func Test_Prepare_SendsTheCommandID(t *testing.T) {
	c, got := jsonAPI(t, "/v2/interactive-submission/prepare", http.StatusOK,
		`{"preparedTransaction":"tx","preparedTransactionHash":"aGFzaA==","hashingSchemeVersion":"HASHING_SCHEME_VERSION_V2"}`)

	prepared, err := c.Prepare(t.Context(), "alice::1220", "allocate:s-1:alice::1220:CBTC", domain.Command{
		Commands: []json.RawMessage{json.RawMessage(`{"ExerciseCommand":{}}`)}, Disclosed: []domain.Disclosed{disclosed()},
		SynchronizerID: "sync-1",
	})
	require.NoError(t, err)
	require.Equal(t, domain.Prepared{
		Transaction: "tx", HashingSchemeVersion: "HASHING_SCHEME_VERSION_V2",
		Hash: []byte("hash"),
	}, prepared)

	var body struct {
		CommandID string   `json:"commandId"`
		ActAs     []string `json:"actAs"`
	}
	require.NoError(t, json.Unmarshal(*got, &body))
	require.Equal(t, "allocate:s-1:alice::1220:CBTC", body.CommandID)
	require.Equal(t, []string{"alice::1220"}, body.ActAs)
}

func Test_Execute_WaitsForTheCommit(t *testing.T) {
	c, got := jsonAPI(t, "/v2/interactive-submission/executeAndWait", http.StatusOK,
		`{"updateId":"1220ab","completionOffset":42}`)

	err := c.Execute(t.Context(), domain.Prepared{Transaction: "tx", HashingSchemeVersion: "V2", Hash: []byte("h")},
		"alice::1220fp", []byte{1, 2, 3})
	require.NoError(t, err)

	var body struct {
		PreparedTransaction string `json:"preparedTransaction"`
		PartySignatures     struct {
			Signatures []struct {
				Party      string `json:"party"`
				Signatures []struct {
					Signature string `json:"signature"`
					SignedBy  string `json:"signedBy"`
				} `json:"signatures"`
			} `json:"signatures"`
		} `json:"partySignatures"`
	}
	require.NoError(t, json.Unmarshal(*got, &body))
	require.Equal(t, "tx", body.PreparedTransaction)
	require.Equal(t, "alice::1220fp", body.PartySignatures.Signatures[0].Party)
	require.Equal(t, base64.StdEncoding.EncodeToString([]byte{1, 2, 3}), body.PartySignatures.Signatures[0].Signatures[0].Signature)
	require.Equal(t, "1220fp", body.PartySignatures.Signatures[0].Signatures[0].SignedBy)
}

func Test_LedgerErrors_Classified(t *testing.T) {
	for name, tc := range map[string]struct {
		status int
		body   string
		kinds  []error
		not    []error
	}{
		"duplicate command": {
			http.StatusConflict, `{"code":"DUPLICATE_COMMAND","cause":"Command submission already exists.","errorCategory":10}`,
			[]error{domain.ErrLedger, domain.ErrDuplicateCommand},
			[]error{domain.ErrLedgerUnavailable, domain.ErrContractGone},
		},
		"contract not found": {
			http.StatusNotFound, `{"code":"CONTRACT_NOT_FOUND","cause":"Contract could not be found","errorCategory":11}`,
			[]error{domain.ErrLedger, domain.ErrContractGone},
			[]error{domain.ErrLedgerUnavailable},
		},
		"inactive contracts": {
			http.StatusConflict, `{"code":"LOCAL_VERDICT_INACTIVE_CONTRACTS","cause":"inactive","errorCategory":2}`,
			[]error{domain.ErrLedger, domain.ErrContractGone},
			[]error{domain.ErrLedgerUnavailable},
		},
		"signature by another key": {
			http.StatusBadRequest, `{"code":"FAILED_TO_EXECUTE_TRANSACTION","cause":"The participant ` +
				`failed to execute the transaction: Received 0 valid signatures from distinct keys (1 invalid), but expected at ` +
				`least 1 valid for alice::1220…. Transaction hash to be signed: 1220e8f0. Ensure the correct transaction hash is ` +
				`signed with the correct key(s).","errorCategory":8}`,
			[]error{domain.ErrSignatureRejected},
			[]error{domain.ErrLedgerUnavailable, domain.ErrContractGone, domain.ErrDuplicateCommand},
		},
		"execution failed otherwise": {
			http.StatusBadRequest, `{"code":"FAILED_TO_EXECUTE_TRANSACTION","cause":"The participant ` +
				`failed to execute the transaction: the prepared transaction is malformed","errorCategory":8}`,
			[]error{domain.ErrLedger},
			[]error{domain.ErrSignatureRejected, domain.ErrLedgerUnavailable},
		},
		"transient category": {
			http.StatusConflict, `{"code":"SUBMISSION_ALREADY_IN_FLIGHT","cause":"in flight","errorCategory":2}`,
			[]error{domain.ErrLedger, domain.ErrLedgerUnavailable},
			nil,
		},
		"unavailable category": {
			http.StatusServiceUnavailable, `{"code":"SERVICE_NOT_RUNNING","cause":"starting","errorCategory":1}`,
			[]error{domain.ErrLedger, domain.ErrLedgerUnavailable},
			nil,
		},
		"deadline category": {
			http.StatusGatewayTimeout, `{"code":"REQUEST_TIME_OUT","cause":"timed out","errorCategory":3}`,
			[]error{domain.ErrLedger, domain.ErrLedgerUnavailable},
			nil,
		},
		"5xx without a body": {
			http.StatusBadGateway, `upstream connect error`,
			[]error{domain.ErrLedger, domain.ErrLedgerUnavailable},
			nil,
		},
		"4xx without a body": {
			http.StatusBadRequest, `Invalid value for: body`,
			[]error{domain.ErrLedger},
			[]error{domain.ErrLedgerUnavailable},
		},
	} {
		t.Run(name, func(t *testing.T) {
			c, _ := jsonAPI(t, "/v2/interactive-submission/executeAndWait", tc.status, tc.body)

			err := c.Execute(t.Context(), domain.Prepared{}, "alice::1220", nil)
			for _, kind := range tc.kinds {
				require.ErrorIs(t, err, kind)
			}

			for _, kind := range tc.not {
				require.NotErrorIs(t, err, kind)
			}
		})
	}
}

func Test_TransportFailureIsUnavailable(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()

	err := New(Config{URL: url, UserID: "desk"}).Execute(t.Context(), domain.Prepared{}, "alice::1220", nil)
	require.ErrorIs(t, err, domain.ErrLedger)
	require.ErrorIs(t, err, domain.ErrLedgerUnavailable)
}

func Test_ResolveParty_Ambiguous(t *testing.T) {
	c, _ := jsonAPI(t, "/v2/parties", http.StatusOK,
		`{"partyDetails":[{"party":"makerA::1220aa"},{"party":"makerA::1220bb"}],"nextPageToken":""}`)

	_, err := c.ResolveParty(t.Context(), "makerA")
	require.ErrorIs(t, err, domain.ErrAmbiguousParty)
	require.ErrorContains(t, err, "makerA::1220aa, makerA::1220bb")
}

func disclosed() domain.Disclosed {
	return domain.Disclosed{TemplateID: "pkg:Rules", ContractID: "rules-cid", CreatedEventBlob: "blob", SynchronizerID: "sync-1"}
}
