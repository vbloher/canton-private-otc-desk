package canton_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/pkg/decimal"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/adapter/canton"
	"github.com/vbloher/canton-private-otc-desk/services/ledger/internal/domain"
)

//nolint:gochecknoglobals
var (
	rules = domain.Disclosed{
		TemplateID:       "pkg:Splice.Testing.Tokens.TestTokenV2:TokenRules",
		ContractID:       "rules-cid",
		CreatedEventBlob: "rules-blob",
		SynchronizerID:   "sync-1",
	}
	tokens = []domain.Disclosed{
		{TemplateID: "pkg:Splice.Testing.Tokens.TestTokenV2.Holding:Token", ContractID: "token-1", CreatedEventBlob: "blob-1", SynchronizerID: "sync-1"},
		{TemplateID: "pkg:Splice.Testing.Tokens.TestTokenV2.Holding:Token", ContractID: "token-2", CreatedEventBlob: "blob-2", SynchronizerID: "sync-1"},
	}
	noon = time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
)

const rulesContext = `{"context":{"values":{"testTokenV2/tokenRules":{"tag":"AV_ContractId","value":"rules-cid"}}},"meta":{"values":{}}}`

func client() *canton.Canton {
	return canton.New(canton.Config{URL: "http://ledger.invalid", UserID: "participant_admin"})
}

func tradeLegs() []domain.TransferLeg {
	return []domain.TransferLeg{
		{ID: "base", Sender: "makerA", Receiver: "taker", Instrument: "CBTC", Amount: decimal.MustParse("1.0")},
		{ID: "quote", Sender: "taker", Receiver: "makerA", Instrument: "USDCx", Amount: decimal.MustParse("100000.0")},
		{ID: "fee", Sender: "makerA", Receiver: "operator", Instrument: "USDCx", Amount: decimal.MustParse("100.0")},
	}
}

func Test_AllocateCommand_JSON(t *testing.T) {
	plan := domain.Plan(tradeLegs(), "operator")
	makerUSDCx := domain.SlotsOf(plan, "makerA")[1]

	cmd, err := client().AllocateCommand(rules, domain.AllocateSpec{
		Operator:     "operator",
		SettlementID: "settlement-1",
		Slot:         makerUSDCx,
		Deadline:     noon.Add(30 * time.Minute),
		RequestedAt:  noon,
		Inputs:       []string{"holding-1", "holding-2"},
	})
	require.NoError(t, err)

	require.JSONEq(t, `{"ExerciseCommand":{
		"templateId":"#splice-api-token-allocation-instruction-v2:Splice.Api.Token.AllocationInstructionV2:AllocationFactory",
		"contractId":"rules-cid",
		"choice":"AllocationFactory_Allocate",
		"choiceArgument":{
			"settlement":{"executors":["operator"],"id":"settlement-1","cid":null,"meta":{"values":{}}},
			"allocation":{
				"admin":"operator",
				"authorizer":{"owner":"makerA","provider":null,"id":""},
				"transferLegSides":[
					{"transferLegId":"quote","side":"ReceiverSide","otherside":{"owner":"taker","provider":null,"id":""},
					 "amount":"100000.0","instrumentId":"USDCx","meta":{"values":{}}},
					{"transferLegId":"fee","side":"SenderSide","otherside":{"owner":"operator","provider":null,"id":""},
					 "amount":"100.0","instrumentId":"USDCx","meta":{"values":{}}}
				],
				"settlementDeadline":"2026-10-21T12:30:00Z",
				"nextIterationFunding":null,
				"committed":true,
				"meta":{"values":{}}
			},
			"requestedAt":"2026-10-21T12:00:00Z",
			"inputHoldingCids":["holding-1","holding-2"],
			"extraArgs":`+rulesContext+`,
			"actors":["makerA"]
		}
	}}`, string(cmd.Payload))
	require.Equal(t, []domain.Disclosed{rules}, cmd.Disclosed)
	require.Equal(t, "sync-1", cmd.SynchronizerID)

	// A pure receiver funds nothing: its inputs are an empty list, never null.
	operatorSlot := domain.SlotsOf(plan, "operator")[0]

	cmd, err = client().AllocateCommand(rules, domain.AllocateSpec{
		Operator: "operator", SettlementID: "settlement-1", Slot: operatorSlot,
		Deadline: noon.Add(30 * time.Minute), RequestedAt: noon,
	})
	require.NoError(t, err)
	require.Contains(t, string(cmd.Payload), `"inputHoldingCids":[]`)
	require.Contains(t, string(cmd.Payload), `"actors":["operator"]`)
}

func Test_SettleCommand_JSON(t *testing.T) {
	cmd, err := client().SettleCommand(rules, tokens, domain.SettleSpec{
		Operator:       "operator",
		SettlementID:   "settlement-1",
		Legs:           tradeLegs(),
		AllocationCIDs: []string{"alloc-1", "alloc-2"},
	})
	require.NoError(t, err)

	require.JSONEq(t, `{"ExerciseCommand":{
		"templateId":"#splice-api-token-allocation-v2:Splice.Api.Token.AllocationV2:SettlementFactory",
		"contractId":"rules-cid",
		"choice":"SettlementFactory_SettleBatch",
		"choiceArgument":{
			"settlement":{"executors":["operator"],"id":"settlement-1","cid":null,"meta":{"values":{}}},
			"transferLegs":[
				{"transferLegId":"base","sender":{"owner":"makerA","provider":null,"id":""},
				 "receiver":{"owner":"taker","provider":null,"id":""},"amount":"1.0","instrumentId":"CBTC","meta":{"values":{}}},
				{"transferLegId":"quote","sender":{"owner":"taker","provider":null,"id":""},
				 "receiver":{"owner":"makerA","provider":null,"id":""},"amount":"100000.0","instrumentId":"USDCx","meta":{"values":{}}},
				{"transferLegId":"fee","sender":{"owner":"makerA","provider":null,"id":""},
				 "receiver":{"owner":"operator","provider":null,"id":""},"amount":"100.0","instrumentId":"USDCx","meta":{"values":{}}}
			],
			"allocations":[
				{"allocationCid":"alloc-1","extraTransferLegSides":[],"nextIterationFunding":null},
				{"allocationCid":"alloc-2","extraTransferLegSides":[],"nextIterationFunding":null}
			],
			"actors":["operator"],
			"extraArgs":`+rulesContext+`
		}
	}}`, string(cmd.Payload))
	require.Equal(t, append([]domain.Disclosed{rules}, tokens...), cmd.Disclosed)
	require.Equal(t, "sync-1", cmd.SynchronizerID)
}

func Test_CancelCommand_JSON(t *testing.T) {
	cmd, err := client().CancelCommand(rules, tokens, "alloc-1", "operator")
	require.NoError(t, err)

	require.JSONEq(t, `{"ExerciseCommand":{
		"templateId":"#splice-api-token-allocation-v2:Splice.Api.Token.AllocationV2:Allocation",
		"contractId":"alloc-1",
		"choice":"Allocation_Cancel",
		"choiceArgument":{"actors":["operator"],"extraArgs":`+rulesContext+`}
	}}`, string(cmd.Payload))
	require.Equal(t, append([]domain.Disclosed{rules}, tokens...), cmd.Disclosed)
}

func Test_OfferMintCommand_JSON(t *testing.T) {
	cmd, err := client().OfferMintCommand(rules, domain.OfferMintSpec{
		Admin:        "operator",
		Receiver:     "alice",
		InstrumentID: "CBTC",
		Amount:       decimal.MustParse("5"),
		OfferedAt:    noon.Add(-time.Minute),
	})
	require.NoError(t, err)

	require.JSONEq(t, `{"ExerciseCommand":{
		"templateId":"#splice-test-token-v2:Splice.Testing.Tokens.TestTokenV2:TokenRules",
		"contractId":"rules-cid",
		"choice":"TokenRules_OfferMint",
		"choiceArgument":{
			"receiver":{"owner":"alice","provider":null,"id":""},
			"amount":"5.0",
			"instrumentId":{"admin":"operator","id":"CBTC"},
			"offeredAt":"2026-10-21T11:59:00Z",
			"receiverConfig":{
				"admin":"operator",
				"account":{"owner":"alice","provider":null,"id":""},
				"ownerConfig":{"canInitiate":true,"mustApprove":true},
				"providerConfig":{"canInitiate":false,"mustApprove":false}
			}
		}
	}}`, string(cmd.Payload))
	require.Equal(t, []domain.Disclosed{rules}, cmd.Disclosed)
	require.Equal(t, "sync-1", cmd.SynchronizerID)
}

func Test_AcceptTransferCommand_JSON(t *testing.T) {
	// The accept consumes the sender's locked holding, which the receiver cannot see.
	cmd, err := client().AcceptTransferCommand(rules, tokens[:1], "offer-cid")
	require.NoError(t, err)

	require.JSONEq(t, `{"ExerciseCommand":{
		"templateId":"#splice-api-token-transfer-instruction-v1:Splice.Api.Token.TransferInstructionV1:TransferInstruction",
		"contractId":"offer-cid",
		"choice":"TransferInstruction_Accept",
		"choiceArgument":{"extraArgs":`+rulesContext+`}
	}}`, string(cmd.Payload))
	require.Equal(t, []domain.Disclosed{rules, tokens[0]}, cmd.Disclosed)
	require.Equal(t, "sync-1", cmd.SynchronizerID)

	// A mint offer has no inputs: TokenRules alone.
	cmd, err = client().AcceptTransferCommand(rules, nil, "offer-cid")
	require.NoError(t, err)
	require.Equal(t, []domain.Disclosed{rules}, cmd.Disclosed)
}

func Test_TransferCommand_JSON(t *testing.T) {
	cmd, err := client().TransferCommand(rules, domain.TransferSpec{
		Admin:         "operator",
		Sender:        "alice",
		Receiver:      "bob",
		InstrumentID:  "CBTC",
		Amount:        decimal.MustParse("1.5"),
		Inputs:        []string{"holding-1"},
		RequestedAt:   noon,
		ExecuteBefore: noon.Add(24 * time.Hour),
	})
	require.NoError(t, err)

	require.JSONEq(t, `{"ExerciseCommand":{
		"templateId":"#splice-api-token-transfer-instruction-v1:Splice.Api.Token.TransferInstructionV1:TransferFactory",
		"contractId":"rules-cid",
		"choice":"TransferFactory_Transfer",
		"choiceArgument":{
			"expectedAdmin":"operator",
			"transfer":{
				"sender":"alice",
				"receiver":"bob",
				"amount":"1.5",
				"instrumentId":{"admin":"operator","id":"CBTC"},
				"requestedAt":"2026-10-21T12:00:00Z",
				"executeBefore":"2026-10-22T12:00:00Z",
				"inputHoldingCids":["holding-1"],
				"meta":{"values":{}}
			},
			"extraArgs":`+rulesContext+`
		}
	}}`, string(cmd.Payload))
	require.Equal(t, []domain.Disclosed{rules}, cmd.Disclosed)
}

func Test_LedgerTime_Microseconds(t *testing.T) {
	cmd, err := client().OfferMintCommand(rules, domain.OfferMintSpec{
		Admin: "operator", Receiver: "alice", InstrumentID: "CBTC", Amount: decimal.MustParse("1"),
		OfferedAt: time.Date(2026, 10, 21, 12, 0, 0, 123456789, time.FixedZone("CEST", 2*60*60)),
	})
	require.NoError(t, err)
	require.Contains(t, string(cmd.Payload), `"offeredAt":"2026-10-21T10:00:00.123456Z"`)
}
