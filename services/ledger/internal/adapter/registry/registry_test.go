package registry_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/pkg/decimal"
	"github.com/vbloher/canton-private-otc-desk/pkg/otel"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/adapter/registry"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/domain"
)

var noon = time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC) //nolint:gochecknoglobals

const factoryAnswer = `{
	"factoryId":"factory-cid",
	"transferKind":"offer",
	"choiceContext":{
		"choiceContextData":{"values":{"rules":{"tag":"AV_ContractId","value":"rules-cid"}}},
		"disclosedContracts":[
			{"templateId":"pkg:Rules","contractId":"rules-cid","createdEventBlob":"blob","synchronizerId":"sync-1"}
		]
	}
}`

const contextAnswer = `{
	"choiceContextData":{"values":{"lock":{"tag":"AV_ContractId","value":"lock-cid"}}},
	"disclosedContracts":[
		{"templateId":"pkg:Holding","contractId":"holding-cid","createdEventBlob":"blob-h","synchronizerId":"sync-1"}
	]
}`

type fakeRegistry struct {
	mu      sync.Mutex
	srv     *httptest.Server
	answers map[string]response
	bodies  map[string][]byte
}

type response struct {
	status int
	body   string
}

func newRegistry(t *testing.T, answers map[string]response) *fakeRegistry {
	t.Helper()

	otel.SilentModeInit()

	f := &fakeRegistry{answers: answers, bodies: map[string][]byte{}}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)

		f.mu.Lock()
		f.bodies[r.URL.EscapedPath()] = body
		f.mu.Unlock()

		answer, ok := f.answers[r.URL.EscapedPath()]
		if !ok || r.Method != http.MethodPost {
			http.NotFound(w, r)

			return
		}

		w.WriteHeader(answer.status)
		_, _ = io.WriteString(w, answer.body)
	}))
	t.Cleanup(f.srv.Close)

	return f
}

func (f *fakeRegistry) root(admin string) string {
	return f.srv.URL + "/api/token-standard/v0/registrars/" + admin + "/registry"
}

func (f *fakeRegistry) body(t *testing.T, path string) map[string]json.RawMessage {
	t.Helper()

	f.mu.Lock()
	defer f.mu.Unlock()

	var out map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(f.bodies[path], &out))

	return out
}

func cbtc(root string) domain.Instrument {
	return domain.Instrument{Symbol: "CBTC", ID: "CBTC", Admin: "cbtc::1220", Registry: root}
}

func cc(root string) domain.Instrument {
	return domain.Instrument{Symbol: "CC", ID: "Amulet", Admin: "DSO::1220", Registry: root}
}

func Test_AllocateCommand_UsesTheRegistryFactory(t *testing.T) {
	f := newRegistry(t, map[string]response{
		"/api/token-standard/v0/registrars/cbtc::1220/registry/allocation-instruction/v2/allocation-factory": {
			status: http.StatusOK, body: factoryAnswer,
		},
	})

	legs := []domain.TransferLeg{
		{ID: "base", Sender: "maker", Receiver: "taker", Instrument: cbtc(f.root("cbtc::1220")),
			Amount: decimal.MustParse("1.0")},
	}
	slot := domain.SlotsOf(domain.Plan(legs), "maker")[0]

	cmd, err := registry.New().AllocateCommand(t.Context(), domain.AllocateSpec{
		Operator:     "operator",
		SettlementID: "settlement-1",
		Slot:         slot,
		Deadline:     noon.Add(30 * time.Minute),
		RequestedAt:  noon,
		Inputs:       []string{"holding-1"},
	})
	require.NoError(t, err)

	require.Len(t, cmd.Commands, 1)
	require.JSONEq(t, `{"ExerciseCommand":{
		"templateId":"#splice-api-token-allocation-instruction-v2:Splice.Api.Token.AllocationInstructionV2:AllocationFactory",
		"contractId":"factory-cid",
		"choice":"AllocationFactory_Allocate",
		"choiceArgument":{
			"settlement":{"executors":["operator"],"id":"settlement-1","cid":null,"meta":{"values":{}}},
			"allocation":{
				"admin":"cbtc::1220",
				"authorizer":{"owner":"maker","provider":null,"id":""},
				"transferLegSides":[
					{"transferLegId":"base","side":"SenderSide","otherside":{"owner":"taker","provider":null,"id":""},
					 "amount":"1.0","instrumentId":"CBTC","meta":{"values":{}}}
				],
				"settlementDeadline":"2026-10-21T12:30:00Z",
				"nextIterationFunding":null,
				"committed":true,
				"meta":{"values":{}}
			},
			"requestedAt":"2026-10-21T12:00:00Z",
			"inputHoldingCids":["holding-1"],
			"extraArgs":{"context":{"values":{"rules":{"tag":"AV_ContractId","value":"rules-cid"}}},"meta":{"values":{}}},
			"actors":["maker"]
		}
	}}`, string(cmd.Commands[0]))
	require.Equal(t, []domain.Disclosed{
		{TemplateID: "pkg:Rules", ContractID: "rules-cid", CreatedEventBlob: "blob", SynchronizerID: "sync-1"},
	}, cmd.Disclosed)
	require.Equal(t, "sync-1", cmd.SynchronizerID)

	sent := f.body(t, "/api/token-standard/v0/registrars/cbtc::1220/registry/allocation-instruction/v2/allocation-factory")
	require.JSONEq(t, `true`, string(sent["excludeDebugFields"]))
	require.Contains(t, string(sent["choiceArguments"]), `"extraArgs":{"context":{"values":{}},"meta":{"values":{}}}`)
	require.Contains(t, string(sent["choiceArguments"]), `"inputHoldingCids":["holding-1"]`)
}

func Test_SettleCommand_OneBatchPerAdmin(t *testing.T) {
	ccFactory := `{"factoryId":"cc-factory","choiceContext":{"choiceContextData":{"values":{}},"disclosedContracts":[
		{"templateId":"pkg:AmuletRules","contractId":"amulet-rules","createdEventBlob":"blob-a","synchronizerId":"sync-1"},
		{"templateId":"pkg:Rules","contractId":"rules-cid","createdEventBlob":"blob","synchronizerId":"sync-1"}
	]}}`
	f := newRegistry(t, map[string]response{
		"/api/token-standard/v0/registrars/cbtc::1220/registry/allocation/v2/settlement-factory": {
			status: http.StatusOK, body: factoryAnswer,
		},
		"/api/token-standard/v0/registrars/DSO::1220/registry/allocation/v2/settlement-factory": {
			status: http.StatusOK, body: ccFactory,
		},
	})

	base := domain.TransferLeg{ID: "base", Sender: "maker", Receiver: "taker",
		Instrument: cbtc(f.root("cbtc::1220")), Amount: decimal.MustParse("1.0")}
	quote := domain.TransferLeg{ID: "quote", Sender: "taker", Receiver: "maker",
		Instrument: cc(f.root("DSO::1220")), Amount: decimal.MustParse("500000.0")}

	cmd, err := registry.New().SettleCommand(t.Context(), domain.SettleSpec{
		Operator:     "operator",
		SettlementID: "settlement-1",
		Batches: []domain.Batch{
			{Admin: "cbtc::1220", Registry: f.root("cbtc::1220"), Legs: []domain.TransferLeg{base},
				AllocationCIDs: []string{"alloc-maker", "alloc-taker-recv"}},
			{Admin: "DSO::1220", Registry: f.root("DSO::1220"), Legs: []domain.TransferLeg{quote},
				AllocationCIDs: []string{"alloc-taker"}},
		},
	})
	require.NoError(t, err)

	require.Len(t, cmd.Commands, 2)
	require.JSONEq(t, `{"ExerciseCommand":{
		"templateId":"#splice-api-token-allocation-v2:Splice.Api.Token.AllocationV2:SettlementFactory",
		"contractId":"cc-factory",
		"choice":"SettlementFactory_SettleBatch",
		"choiceArgument":{
			"settlement":{"executors":["operator"],"id":"settlement-1","cid":null,"meta":{"values":{}}},
			"transferLegs":[
				{"transferLegId":"quote","sender":{"owner":"taker","provider":null,"id":""},
				 "receiver":{"owner":"maker","provider":null,"id":""},"amount":"500000.0","instrumentId":"Amulet",
				 "meta":{"values":{}}}
			],
			"allocations":[{"allocationCid":"alloc-taker","extraTransferLegSides":[],"nextIterationFunding":null}],
			"actors":["operator"],
			"extraArgs":{"context":{"values":{}},"meta":{"values":{}}}
		}
	}}`, string(cmd.Commands[1]))
	require.Contains(t, string(cmd.Commands[0]), `"contractId":"factory-cid"`)
	require.Equal(t, []string{"rules-cid", "amulet-rules"}, contractIDs(cmd.Disclosed))
	require.Equal(t, "sync-1", cmd.SynchronizerID)
}

func Test_SettleCommand_RefusesSeveralSynchronizers(t *testing.T) {
	other := `{"factoryId":"cc-factory","choiceContext":{"choiceContextData":{"values":{}},"disclosedContracts":[
		{"templateId":"pkg:AmuletRules","contractId":"amulet-rules","createdEventBlob":"blob-a","synchronizerId":"sync-2"}
	]}}`
	f := newRegistry(t, map[string]response{
		"/a/registry/allocation/v2/settlement-factory": {status: http.StatusOK, body: factoryAnswer},
		"/b/registry/allocation/v2/settlement-factory": {status: http.StatusOK, body: other},
	})

	_, err := registry.New().SettleCommand(t.Context(), domain.SettleSpec{
		Operator: "operator", SettlementID: "s-1",
		Batches: []domain.Batch{
			{Admin: "a::1", Registry: f.srv.URL + "/a/registry"},
			{Admin: "b::1", Registry: f.srv.URL + "/b/registry"},
		},
	})
	require.ErrorIs(t, err, domain.ErrLedger)
	require.ErrorContains(t, err, "sync-1, sync-2")
}

func Test_CancelCommand_UsesTheChoiceContext(t *testing.T) {
	f := newRegistry(t, map[string]response{
		"/r/registry/allocations/v2/alloc-1/choice-contexts/cancel": {status: http.StatusOK, body: contextAnswer},
	})

	cmd, err := registry.New().CancelCommand(t.Context(), f.srv.URL+"/r/registry", "alloc-1", "operator")
	require.NoError(t, err)

	require.JSONEq(t, `{"ExerciseCommand":{
		"templateId":"#splice-api-token-allocation-v2:Splice.Api.Token.AllocationV2:Allocation",
		"contractId":"alloc-1",
		"choice":"Allocation_Cancel",
		"choiceArgument":{"actors":["operator"],
			"extraArgs":{"context":{"values":{"lock":{"tag":"AV_ContractId","value":"lock-cid"}}},"meta":{"values":{}}}}
	}}`, string(cmd.Commands[0]))
	require.Equal(t, []string{"holding-cid"}, contractIDs(cmd.Disclosed))

	sent := f.body(t, "/r/registry/allocations/v2/alloc-1/choice-contexts/cancel")
	require.JSONEq(t, `{}`, string(sent["meta"]))
}

func Test_AcceptTransferCommand_UsesTheChoiceContext(t *testing.T) {
	f := newRegistry(t, map[string]response{
		"/r/registry/transfer-instruction/v1/offer-1/choice-contexts/accept": {status: http.StatusOK, body: contextAnswer},
	})

	cmd, err := registry.New().AcceptTransferCommand(t.Context(), f.srv.URL+"/r/registry", "offer-1")
	require.NoError(t, err)

	require.JSONEq(t, `{"ExerciseCommand":{
		"templateId":"#splice-api-token-transfer-instruction-v1:Splice.Api.Token.TransferInstructionV1:TransferInstruction",
		"contractId":"offer-1",
		"choice":"TransferInstruction_Accept",
		"choiceArgument":{
			"extraArgs":{"context":{"values":{"lock":{"tag":"AV_ContractId","value":"lock-cid"}}},"meta":{"values":{}}}}
	}}`, string(cmd.Commands[0]))
	require.Equal(t, "sync-1", cmd.SynchronizerID)
}

func Test_TransferCommand_UsesTheTransferFactory(t *testing.T) {
	f := newRegistry(t, map[string]response{
		"/r/registry/transfer-instruction/v1/transfer-factory": {status: http.StatusOK, body: factoryAnswer},
	})

	cmd, err := registry.New().TransferCommand(t.Context(), domain.TransferSpec{
		Registry:      f.srv.URL + "/r/registry",
		Admin:         "cbtc::1220",
		Sender:        "alice",
		Receiver:      "bob",
		InstrumentID:  "CBTC",
		Amount:        decimal.MustParse("1.5"),
		Inputs:        []string{"holding-1"},
		RequestedAt:   time.Date(2026, 10, 21, 12, 0, 0, 123456789, time.FixedZone("CEST", 2*60*60)),
		ExecuteBefore: noon.Add(24 * time.Hour),
	})
	require.NoError(t, err)

	require.JSONEq(t, `{"ExerciseCommand":{
		"templateId":"#splice-api-token-transfer-instruction-v1:Splice.Api.Token.TransferInstructionV1:TransferFactory",
		"contractId":"factory-cid",
		"choice":"TransferFactory_Transfer",
		"choiceArgument":{
			"expectedAdmin":"cbtc::1220",
			"transfer":{
				"sender":"alice",
				"receiver":"bob",
				"amount":"1.5",
				"instrumentId":{"admin":"cbtc::1220","id":"CBTC"},
				"requestedAt":"2026-10-21T10:00:00.123456Z",
				"executeBefore":"2026-10-22T12:00:00Z",
				"inputHoldingCids":["holding-1"],
				"meta":{"values":{}}
			},
			"extraArgs":{"context":{"values":{"rules":{"tag":"AV_ContractId","value":"rules-cid"}}},"meta":{"values":{}}}
		}
	}}`, string(cmd.Commands[0]))
}

func Test_RegistryErrors(t *testing.T) {
	f := newRegistry(t, map[string]response{
		"/r/registry/transfer-instruction/v1/transfer-factory":               {status: http.StatusServiceUnavailable},
		"/r/registry/transfer-instruction/v1/gone/choice-contexts/accept":    {status: http.StatusNotFound},
		"/r/registry/allocations/v2/bad/choice-contexts/cancel":              {status: http.StatusBadRequest, body: "no"},
		"/r/registry/allocation-instruction/v2/allocation-factory":           {status: http.StatusOK, body: `{}`},
		"/r/registry/transfer-instruction/v1/garbled/choice-contexts/accept": {status: http.StatusOK, body: `{`},
	})
	r := registry.New()
	root := f.srv.URL + "/r/registry"

	_, err := r.TransferCommand(t.Context(), domain.TransferSpec{Registry: root})
	require.ErrorIs(t, err, domain.ErrLedgerUnavailable)

	_, err = r.AcceptTransferCommand(t.Context(), root, "gone")
	require.ErrorIs(t, err, domain.ErrContractGone)

	_, err = r.CancelCommand(t.Context(), root, "bad", "operator")
	require.ErrorIs(t, err, domain.ErrLedger)
	require.NotErrorIs(t, err, domain.ErrLedgerUnavailable)
	require.ErrorContains(t, err, "HTTP 400: no")

	_, err = r.AllocateCommand(t.Context(), domain.AllocateSpec{Slot: domain.Slot{Registry: root}})
	require.ErrorIs(t, err, domain.ErrLedger)
	require.ErrorContains(t, err, "without a factory")

	_, err = r.AcceptTransferCommand(t.Context(), root, "garbled")
	require.ErrorIs(t, err, domain.ErrLedger)

	_, err = r.AcceptTransferCommand(t.Context(), "http://127.0.0.1:1/registry", "offer")
	require.ErrorIs(t, err, domain.ErrLedgerUnavailable)
}

func contractIDs(disclosed []domain.Disclosed) []string {
	ids := make([]string, 0, len(disclosed))
	for _, d := range disclosed {
		ids = append(ids, d.ContractID)
	}

	return ids
}
